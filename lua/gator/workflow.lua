local health = require("gator.health")
local managed_adapter = require("gator.adapters.managed")
local native_terminal = require("gator.adapters.native_terminal")
local structured = require("gator.adapters.structured")
local terminal = require("gator.adapters.terminal")
local run_store = require("gator.run_store")
local capture = require("gator.context.capture")
local provider_picker = require("gator.ui.provider_picker")
local conversation = require("gator.ui.conversation")
local run_graph = require("gator.ui.run_graph")
local handoff_review = require("gator.ui.run_handoff")
local review_ui = require("gator.ui.run_review")
local loading_ui = require("gator.ui.loading")
local worktree = require("gator.workspace.worktree")
local workspace_cleanup = require("gator.workspace.cleanup")
local retention = require("gator.core.retention")
local resources = require("gator.core.resources")
local retention_ui = require("gator.ui.retention")
local context_preflight = require("gator.ui.context_preflight")
local run_events = require("gator.ui.run_events")
local state_store = require("gator.state")
local review_validation = require("gator.review.validation")
local policy_overlay = require("gator.policy.overlay")
local trust = require("gator.trust")
local extension_runtime = require("gator.extensions.runtime")

local M = { name = "workflow", api_version = 2 }
local Workflow = {}
Workflow.__index = Workflow

local function fail(message)
	error("Gator workflow: " .. tostring(message), 3)
end

local function text(value, name)
	if type(value) ~= "string" or vim.trim(value) == "" then
		fail(name .. " must be non-empty text")
	end
	return value
end

local function provider(value)
	value = text(value, "provider")
	if not value:match("^[a-z][a-z0-9_-]*$") then
		fail("provider must be a lowercase identifier")
	end
	return value
end

local function project_root(value)
	if value then
		local resolved = vim.uv.fs_realpath(value)
		if resolved and vim.fn.isdirectory(resolved) == 1 then
			return resolved
		end
		fail("root must resolve to a directory")
	end
	local result = vim.system({ "git", "rev-parse", "--show-toplevel" }, { cwd = vim.fn.getcwd(), text = true }):wait()
	if result.code ~= 0 then
		fail("current directory is not a Git workspace")
	end
	return project_root(vim.trim(result.stdout or ""))
end

local function now()
	return os.time()
end

local function failure_code(value)
	local code = type(value) == "table" and value.code or nil
	if type(code) == "string" and code:match("^[a-z][a-z0-9_.-]*$") then
		return code
	end
	return "provider_error"
end

local function active_state(value)
	return value.state == "starting"
		or value.state == "running"
		or value.state == "waiting_input"
		or value.state == "detached"
end

local roles = {
	primary = true,
	researcher = true,
	writer = true,
	reviewer = true,
	integrator = true,
}

local function role(value)
	value = value or "primary"
	if type(value) ~= "string" or not roles[value] then
		fail("run role is unavailable")
	end
	return value
end

local function writers(value)
	return active_state(value) and (value.role == "primary" or value.role == "writer" or value.role == "integrator")
end

local function role_instruction(value)
	if value == "researcher" then
		return "Gator role: researcher. Inspect and report; do not edit files or run mutating commands."
	end
	if value == "reviewer" then
		return "Gator role: reviewer. Inspect the diff and collect evidence; do not edit source files."
	end
	if value == "writer" then
		return "Gator role: writer. Make changes only in this isolated Gator worktree."
	end
	if value == "integrator" then
		return "Gator role: integrator. The user explicitly approved convergence; reconcile dependency evidence only in this isolated Gator worktree."
	end
	return nil
end

local function context_kind(value)
	if value ~= "selection" and value ~= "diagnostic" and value ~= "hunk" and value ~= "bundle" then
		fail("context kind must be selection, diagnostic, hunk, or bundle")
	end
	return value
end

function M.new(opts)
	opts = opts or {}
	if type(opts) ~= "table" or not state_store.is(opts.state) then
		fail("new requires initialized state")
	end
	for key in pairs(opts) do
		if
			key ~= "state"
			and key ~= "root"
			and key ~= "terminal"
			and key ~= "bridge"
			and key ~= "readiness"
			and key ~= "managed"
			and key ~= "structured"
			and key ~= "loading"
			and key ~= "handoff_review"
			and key ~= "review_ui"
			and key ~= "review_validation"
			and key ~= "retention_ui"
			and key ~= "extensions"
			and key ~= "clock"
		then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	if opts.clock ~= nil and type(opts.clock) ~= "function" then
		fail("clock must be a function")
	end
	local root = project_root(opts.root)
	local value = setmetatable({
		state = opts.state,
		root = root,
		store = run_store.new(root),
		terminal = opts.terminal or terminal.new(),
		bridge = opts.bridge or native_terminal.new(),
		readiness = opts.readiness or health.launch_catalog,
		managed = opts.managed or managed_adapter.new({ shutdown = true }),
		structured = opts.structured or structured.new(),
		loading = opts.loading or loading_ui,
		handoff_review = opts.handoff_review or handoff_review,
		review_ui = opts.review_ui or review_ui,
		review_validation = opts.review_validation or review_validation,
		retention_ui = opts.retention_ui or retention_ui,
		extensions = opts.extensions or extension_runtime.new({ modules = {}, renderers = {}, columns = {} }),
		clock = opts.clock or now,
		loading_handles = {},
		providers = {},
		active = {},
		transcripts = {},
		pending_summary = {},
		review_sessions = {},
	}, Workflow)
	value:refresh()
	return value
end

function Workflow:runs()
	return self.store:list()
end

function Workflow:worktree_lease(id)
	return self.store:worktree_lease(id)
end

function Workflow:recover()
	local recovered = 0
	for _, run in ipairs(self:runs()) do
		if active_state(run) and run.state ~= "detached" then
			self:update(run.id, { state = "detached" })
			self:journal(run.id, "run.recovered", { previous_state = run.state, state = "detached" })
			recovered = recovered + 1
		end
	end
	self:reconcile_worktree_leases()
	self:startup_retention()
	return recovered
end

function Workflow:run(id)
	local value = self.store:get(id)
	if not value then
		fail("run is unavailable: " .. tostring(id))
	end
	return value
end

function Workflow:put(value)
	return self.store:put(value)
end

function Workflow:journal(id, event_type, payload)
	local value = self.store:append_event(id, event_type, payload, self.clock())
	local events = {
		["run.created"] = "run.created",
		["context.prepared"] = "context.prepared",
		["context.sent"] = "context.delivered",
		["handoff.prepared"] = "handoff.prepared",
		["handoff.reviewed"] = "handoff.reviewed",
		["handoff.delivered"] = "handoff.delivered",
		["approval.requested"] = "approval.requested",
		["approval.decided"] = "approval.resolved",
	}
	if events[event_type] then
		pcall(
			self.extensions.emit,
			self.extensions,
			events[event_type],
			vim.tbl_extend("force", { run_id = id }, payload or {})
		)
	end
	return value
end

function Workflow:events(id)
	self:run(id)
	return self.store:events(id)
end

function Workflow:open_events(id)
	local run = self:run(id)
	return run_events.open({ run = run, events = self.store:events(run.id) })
end

function Workflow:update(id, patch)
	local value = self:run(id)
	local previous_state = value.state
	local was_active = active_state(value)
	for key, item in pairs(patch) do
		value[key] = vim.deepcopy(item)
	end
	if patch.state ~= nil then
		local active = active_state(value)
		if was_active and not active and value.resources.finished_at == nil then
			value.resources.finished_at = self.clock()
		elseif not was_active and active then
			value.resources.finished_at = nil
		end
	end
	value.updated_at = self.clock()
	local updated = self:put(value)
	if patch.state ~= nil and patch.state ~= previous_state then
		self:journal(updated.id, "run.state", { from = previous_state, to = updated.state })
		pcall(self.extensions.emit, self.extensions, "run.state_changed", {
			run_id = updated.id,
			provider = updated.provider,
			from = previous_state,
			to = updated.state,
		})
		if updated.state == "completed" or updated.state == "failed" or updated.state == "stopped" then
			pcall(self.extensions.emit, self.extensions, "run.finished", {
				run_id = updated.id,
				provider = updated.provider,
				state = updated.state,
			})
		end
	end
	if updated.workspace.kind == "worktree" and not active_state(updated) then
		self:release_worktree_lease(updated)
	end
	pcall(self.extensions.emit, self.extensions, "status.changed", { run_id = updated.id, state = updated.state })
	return updated
end

function Workflow:resource_display()
	return vim.deepcopy(self.state.config.ui.resources)
end

function Workflow:graph_columns()
	return vim.deepcopy(self.state.config.ui.run_graph.columns)
end

function Workflow:render_graph_column(id, run, measurements)
	local builtins = {
		id = run.id,
		provider = run.provider,
		role = run.role,
		state = run.state,
		context = run.bundle_id or "unavailable",
		resources = measurements and (measurements.context_bytes .. " B") or "unknown",
		budget = run.budget.limit_tokens == 0 and "unbounded"
			or (run.budget.state .. " " .. (run.usage.total_tokens or "?") .. "/" .. run.budget.limit_tokens),
		trust = run.trust and run.trust.surface or "unknown",
	}
	if builtins[id] ~= nil then
		return tostring(builtins[id])
	end
	return self.extensions:render_column(id, { run = run, resources = measurements, root = self.root })
end

function Workflow:resource_summary()
	return resources.summary(self:runs())
end

function Workflow:statusline(opts)
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("statusline options must be a table")
	end
	for key in pairs(opts) do
		if key ~= "fields" then
			fail("statusline options contain unsupported field: " .. tostring(key))
		end
	end
	local fields = opts.fields or { "active", "provider", "state" }
	if type(fields) ~= "table" or not vim.islist(fields) then
		fail("statusline fields must be an array")
	end
	local active, current = 0, nil
	for _, run in ipairs(self:runs()) do
		if active_state(run) then
			active = active + 1
			current = current or run
		end
	end
	if active == 0 then
		return "Gator idle"
	end
	local values = {
		active = active .. " active",
		provider = current.provider,
		state = current.state,
		role = current.role,
		budget = current.budget.limit_tokens == 0 and "unbounded"
			or (current.usage.total_tokens or "?") .. "/" .. current.budget.limit_tokens,
	}
	local result = { "Gator" }
	for _, field in ipairs(fields) do
		if type(field) ~= "string" or not values[field] then
			fail("statusline field is unavailable: " .. tostring(field))
		end
		table.insert(result, values[field])
	end
	return table.concat(result, " · ")
end

function Workflow:run_resources(run)
	return resources.run(run, self.clock())
end

function Workflow:record_context_delivery(id, bytes)
	if type(bytes) ~= "number" or bytes < 0 or bytes % 1 ~= 0 then
		fail("context bytes must be a non-negative integer")
	end
	local run = self:run(id)
	local value = vim.deepcopy(run.resources)
	value.context_bytes = value.context_bytes + bytes
	value.context_sends = value.context_sends + 1
	return self:update(id, { resources = value })
end

function Workflow:report_usage(id, usage)
	if type(usage) ~= "table" or usage.state ~= "reported" then
		fail("reported usage is required")
	end
	local run = self:run(id)
	local budget = vim.deepcopy(run.budget)
	if budget.limit_tokens > 0 then
		budget.state = usage.total_tokens and usage.total_tokens >= budget.limit_tokens and "exhausted" or "tracking"
	end
	self:update(id, { usage = usage, budget = budget })
	self:journal(id, "provider.usage", {
		input_tokens = usage.input_tokens,
		output_tokens = usage.output_tokens,
		total_tokens = usage.total_tokens,
		state = usage.state,
	})
	if budget.state == "exhausted" then
		vim.notify(
			"Gator budget reached for " .. run.provider .. " · " .. budget.limit_tokens .. " tokens",
			vim.log.levels.WARN
		)
		if budget.action == "stop" then
			self:cancel(id)
		end
	end
	if run.runbook_id then
		local status = self:runbook_status(run.runbook_id)
		local limit = status.max_tokens > 0 and status.max_tokens or self.state.config.runbooks.max_tokens
		if limit > 0 and status.reported_tokens >= limit then
			vim.notify(
				"Gator runbook reported-token budget reached · " .. status.reported_tokens .. "/" .. limit,
				vim.log.levels.WARN
			)
			if budget.action == "stop" then
				self:cancel(id)
			end
		end
	end
	return budget
end

function Workflow:refresh()
	local providers = {}
	local extension_providers = self.extensions:providers_list()
	local acp_commands = vim.deepcopy(self.state.config.acp.commands)
	for _, definition in ipairs(extension_providers) do
		if definition.kind == "acp" then
			acp_commands[definition.name] = { argv = vim.deepcopy(definition.argv) }
		end
	end
	local terminal_records = self.readiness({
		cwd = self.root,
		pi_user_confirmed = self.state.config.providers.pi.user_confirmed,
	})
	for _, record in ipairs(terminal_records) do
		if record.available and native_terminal.supports(record.provider) then
			providers[record.provider] = {
				provider = record.provider,
				available = true,
				terminal = true,
				chat = structured.supports(record.provider),
				readiness_state = record.readiness_state,
				version = record.version,
			}
		end
	end
	local confirmations = {}
	for name, value in pairs(self.state.config.providers) do
		confirmations[name] = value.user_confirmed
	end
	self.managed:configure({ commands = acp_commands })
	for _, record in
		ipairs(managed_adapter.catalog({
			cwd = self.root,
			user_confirmed = confirmations,
			commands = acp_commands,
		}))
	do
		local definition = self:custom_provider(record.provider)
		local extension_ready = true
		if definition and definition.kind == "acp" then
			local ok, probe = pcall(definition.probe, { cwd = self.root, provider = record.provider })
			local valid = ok and type(probe) == "table" and type(probe.available) == "boolean"
			extension_ready = valid and probe.available
			if not valid and definition.extension then
				self.extensions:disable(
					definition.extension,
					ok and "ACP provider probe must return { available = boolean }" or probe
				)
			end
		end
		if record.available and extension_ready then
			providers[record.provider] = {
				provider = record.provider,
				available = true,
				chat = true,
				terminal = native_terminal.supports(record.provider),
				managed_mode = record.mode,
				readiness_state = record.readiness_state,
				version = record.version,
			}
		end
	end
	for _, definition in ipairs(extension_providers) do
		if definition.kind == "terminal" then
			local ok, probe = pcall(definition.probe, { cwd = self.root, provider = definition.name })
			local valid = ok and type(probe) == "table" and type(probe.available) == "boolean"
			if valid and probe.available then
				providers[definition.name] = {
					provider = definition.name,
					available = true,
					terminal = true,
					chat = false,
					custom = true,
					readiness_state = "configured",
					version = type(probe.version) == "string" and probe.version or nil,
				}
			elseif not valid and definition.extension then
				self.extensions:disable(
					definition.extension,
					ok and "terminal provider probe must return { available = boolean }" or probe
				)
			end
		end
	end
	self.providers = providers
	self.state:update({ adapters = vim.deepcopy(providers) })
	return vim.deepcopy(providers)
end

function Workflow:custom_provider(name)
	return self.extensions:provider(name)
end

function Workflow:prepare_terminal(run, prompt, callback, resume)
	local definition = self:custom_provider(run.provider)
	if definition and definition.kind == "terminal" then
		local operation = resume and definition.resume or definition.start
		if not operation then
			callback(nil, "custom terminal provider does not expose resume")
			return
		end
		local ok, prepared = pcall(operation, {
			provider = run.provider,
			cwd = run.workspace.root,
			prompt = prompt,
			session = vim.deepcopy(run.session),
			run = vim.deepcopy(run),
		})
		if not ok or type(prepared) ~= "table" then
			if definition.extension then
				self.extensions:disable(
					definition.extension,
					ok and "custom terminal provider returned no launch command" or prepared
				)
			end
			callback(nil, ok and "custom terminal provider returned no launch command" or prepared)
			return
		end
		if type(prepared.command) ~= "table" or not vim.islist(prepared.command) or #prepared.command == 0 then
			if definition.extension then
				self.extensions:disable(definition.extension, "custom terminal provider returned an invalid command")
			end
			callback(nil, "custom terminal provider returned an invalid command")
			return
		end
		if type(prepared.session) ~= "table" or type(prepared.session.id) ~= "string" or prepared.session.id == "" then
			if definition.extension then
				self.extensions:disable(definition.extension, "custom terminal provider returned an invalid session")
			end
			callback(nil, "custom terminal provider returned an invalid session")
			return
		end
		prepared.session = { provider = run.provider, id = prepared.session.id, owner = "provider" }
		callback(prepared)
		return
	end
	if resume then
		self.bridge:resume(
			{ provider = run.provider, session = { provider = run.provider, id = run.session.id, owner = "provider" } },
			callback
		)
	else
		self.bridge:start({ provider = run.provider, cwd = run.workspace.root, prompt = prompt }, callback)
	end
end

function Workflow:render_ui(slot, model, fallback)
	local handled, result = self.extensions:render(slot, model)
	if handled then
		return result == nil and true or result
	end
	return fallback()
end

function Workflow:pick_provider(providers, on_launch, on_cancel)
	return self:render_ui("provider_picker", {
		providers = vim.deepcopy(providers),
		actions = {
			select = function(name)
				return on_launch({ provider = provider(name) })
			end,
			cancel = on_cancel,
		},
	}, function()
		return provider_picker.open({ providers = providers, on_launch = on_launch, on_cancel = on_cancel })
	end)
end

function Workflow:open_preflight(preflight, on_confirm, on_cancel)
	return self:render_ui("context_preflight", {
		preflight = vim.deepcopy(preflight),
		actions = { confirm = on_confirm, cancel = on_cancel },
	}, function()
		return context_preflight.open({ preflight = preflight, on_confirm = on_confirm, on_cancel = on_cancel })
	end)
end

function Workflow:provider(name)
	name = provider(name)
	local value = self.providers[name]
	if not value or not value.available then
		fail("provider is unavailable for launch: " .. name)
	end
	return value
end

function Workflow:resolve_transport(record, requested)
	requested = requested or self.state.config.launch.transport
	if requested == "auto" then
		return record.chat and "chat" or "terminal"
	end
	if requested == "chat" and not record.chat then
		fail(record.provider .. " does not expose a documented structured chat transport")
	end
	if requested == "terminal" and not record.terminal then
		fail(record.provider .. " does not expose a native terminal bridge")
	end
	return requested
end

function Workflow:choose(opts)
	opts = opts or {}
	local loading = self.loading.open({ message = "Checking local coding agents" })
	local ok, result = pcall(self.refresh, self)
	loading.close()
	if not ok then
		error(result, 0)
	end
	if opts.provider then
		return self:launch(opts)
	end
	local project = self.store:project()
	local candidate = project.default_provider
	if not candidate and self.state.config.launch.default_provider ~= "ask" then
		candidate = self.state.config.launch.default_provider
	end
	if candidate and self.providers[candidate] then
		opts.provider = candidate
		return self:launch(opts)
	end
	local choices = {}
	for _, value in pairs(self.providers) do
		table.insert(choices, value)
	end
	if #choices == 0 then
		vim.notify("Gator: no ready providers; run :GatorHealth", vim.log.levels.WARN)
		return false
	end
	return self:pick_provider(choices, function(choice)
		opts.provider = choice.provider
		local ok, err = pcall(self.launch, self, opts)
		if not ok then
			vim.notify(tostring(err), vim.log.levels.ERROR, { title = "Gator" })
		end
	end)
end

function Workflow:prompt(opts)
	opts = opts or {}
	local selected = capture.current({ buffer = opts.buffer, first_line = opts.first_line, last_line = opts.last_line })
	local function submit(objective, overrides)
		overrides = overrides or {}
		if type(overrides) ~= "table" then
			fail("dashboard launch overrides must be a table")
		end
		if type(objective) == "string" and vim.trim(objective) ~= "" then
			local ok, err = pcall(self.choose, self, {
				objective = objective,
				capture = selected,
				provider = overrides.provider or opts.provider,
				transport = overrides.transport or opts.transport,
			})
			if not ok then
				vim.notify(tostring(err), vim.log.levels.ERROR, { title = "Gator" })
			end
		end
	end
	return self:render_ui("dashboard", {
		capture = vim.deepcopy(selected),
		actions = { submit = submit, cancel = function() end },
	}, function()
		vim.ui.input({ prompt = "Gator: " }, submit)
		return true
	end)
end

local function git_path(root, argv, name)
	local result = vim.system(argv, { cwd = root, text = true }):wait()
	if result.code ~= 0 or vim.trim(result.stdout or "") == "" then
		fail(name .. " is unavailable")
	end
	local value = vim.trim(result.stdout)
	return value:sub(1, 1) == "/" and vim.fs.normalize(value) or vim.fs.normalize(root .. "/" .. value)
end

local function git_branch(root)
	local result = vim.system({ "git", "branch", "--show-current" }, { cwd = root, text = true }):wait()
	if result.code ~= 0 or vim.trim(result.stdout or "") == "" then
		fail("worktree branch is unavailable")
	end
	return vim.trim(result.stdout)
end

function Workflow:lease_worktree(run)
	if run.workspace.kind ~= "worktree" then
		return nil
	end
	local existing = self.store:worktree_lease(run.id)
	if existing and existing.state == "active" then
		return existing
	end
	local workspace = run.workspace
	local lease = {
		run_id = run.id,
		repository_root = self.root,
		common_git_dir = git_path(self.root, { "git", "rev-parse", "--git-common-dir" }, "Git common directory"),
		worktree_root = workspace.root,
		branch = workspace.branch or git_branch(workspace.root),
		base = workspace.base or capture.head(workspace.root) or "HEAD",
		state = "active",
		created_at = existing and existing.created_at or self.clock(),
		updated_at = self.clock(),
	}
	if not existing then
		worktree.lock({ root = self.root, path = workspace.root, reason = "Gator run " .. run.id })
	elseif existing.state == "released" then
		worktree.lock({ root = self.root, path = workspace.root, reason = "Gator run " .. run.id })
	end
	local ok, value = pcall(self.store.put_worktree_lease, self.store, lease)
	if not ok then
		pcall(worktree.unlock, { root = self.root, path = workspace.root })
		error(value, 0)
	end
	return value
end

function Workflow:release_worktree_lease(run)
	if run.workspace.kind ~= "worktree" then
		return false
	end
	local lease = self.store:worktree_lease(run.id)
	if not lease or lease.state == "released" then
		return false
	end
	local unlocked, reason = pcall(worktree.unlock, { root = lease.repository_root, path = lease.worktree_root })
	if not unlocked then
		vim.notify("Gator worktree unlock: " .. tostring(reason), vim.log.levels.WARN)
		return false
	end
	lease.state, lease.updated_at = "released", self.clock()
	self.store:put_worktree_lease(lease)
	return true
end

function Workflow:reconcile_worktree_leases()
	for _, run in ipairs(self:runs()) do
		if run.workspace.kind == "worktree" then
			local lease = self.store:worktree_lease(run.id)
			if active_state(run) and (not lease or lease.state ~= "active") then
				local ok, reason = pcall(self.lease_worktree, self, run)
				if not ok then
					vim.notify("Gator worktree lease: " .. tostring(reason), vim.log.levels.WARN)
				end
			elseif not active_state(run) and lease and lease.state == "active" then
				self:release_worktree_lease(run)
			end
		end
	end
	return self.store:list_worktree_leases()
end

function Workflow:retention_exclusion()
	local protected = { runs = {}, transcripts = {}, bundles = {}, reviews = {}, handoffs = {}, events = {} }
	for _, run in ipairs(self:runs()) do
		if active_state(run) then
			protected.runs[run.id] = true
			protected.transcripts[run.id] = true
			protected.events[run.id] = true
			protected.reviews[run.id] = true
			if run.bundle_id then
				protected.bundles[run.bundle_id] = true
				protected.handoffs[run.bundle_id] = true
			end
		end
	end
	return function(value, category)
		local name = vim.fn.fnamemodify(value, ":t")
		if category == "runs" or category == "transcripts" or category == "events" then
			return protected[category][name:gsub("%.[^.]+$", "")]
		end
		if category == "bundles" then
			return protected.bundles[name:gsub("%.[^.]+$", "")]
		end
		if category == "reviews" then
			return protected.reviews[vim.fn.fnamemodify(vim.fn.fnamemodify(value, ":h"), ":t")]
		end
		if category == "handoffs" then
			local parent = vim.fn.fnamemodify(value, ":h")
			return protected.handoffs[vim.fn.fnamemodify(parent, ":t")]
		end
		return false
	end
end

function Workflow:worktree_cleanup_plan(opts)
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("worktree cleanup options must be an object")
	end
	local age = self.state.config.retention.max_age_days
	if age == 0 and not opts.ignore_age then
		return { manager = nil, plan = {}, by_path = {} }
	end
	local cutoff = self.clock() - age * 24 * 60 * 60
	local by_path = {}
	for _, lease in ipairs(self.store:list_worktree_leases()) do
		if
			lease.state == "released"
			and (not opts.run_id or lease.run_id == opts.run_id)
			and (opts.ignore_age or lease.updated_at <= cutoff)
		then
			by_path[vim.fs.normalize(lease.worktree_root)] = lease
		end
	end
	if next(by_path) == nil then
		return { manager = nil, plan = {}, by_path = by_path }
	end
	local manager = workspace_cleanup.new({
		root = self.root,
		active = function(path)
			for _, run in ipairs(self:runs()) do
				if active_state(run) and vim.fs.normalize(run.workspace.root) == path then
					return true
				end
			end
			return false
		end,
		owned = function(path)
			return by_path[path] ~= nil
		end,
	})
	return { manager = manager, plan = manager:plan(), by_path = by_path }
end

function Workflow:retention_inventory()
	local manager = retention.project(self.store.directory, self.state.config.retention.max_age_days)
	return manager:inventory()
end

function Workflow:retention_plan(opts)
	opts = opts or {}
	if type(opts) ~= "table" or (opts.quota ~= nil and type(opts.quota) ~= "boolean") then
		fail("retention plan options must provide an optional quota boolean")
	end
	local age = self.state.config.retention.max_age_days
	local manager = retention.project(self.store.directory, age)
	local worktrees = self:worktree_cleanup_plan()
	local exclude = self:retention_exclusion()
	local artifacts = age > 0 and manager:plan(self.clock(), { exclude = exclude }) or {}
	local selected, scheduled_bytes = {}, 0
	for _, artifact in ipairs(artifacts) do
		selected[artifact.path] = true
		scheduled_bytes = scheduled_bytes + (artifact.bytes or 0)
	end
	local quota, inventory = {}, manager:inventory()
	if opts.quota ~= false and self.state.config.retention.max_bytes > 0 then
		quota, inventory = manager:quota_plan(self.state.config.retention.max_bytes, {
			already_scheduled_bytes = scheduled_bytes,
			exclude = function(path, category)
				return selected[path] or exclude(path, category)
			end,
		})
		for _, artifact in ipairs(quota) do
			table.insert(artifacts, artifact)
		end
	end
	local reclaim_bytes = 0
	for _, artifact in ipairs(artifacts) do
		reclaim_bytes = reclaim_bytes + (artifact.bytes or 0)
	end
	return {
		manager = manager,
		artifacts = artifacts,
		worktrees = worktrees,
		by_path = worktrees.by_path,
		inventory = inventory,
		quota_bytes = self.state.config.retention.max_bytes,
		reclaim_bytes = reclaim_bytes,
	}
end

function Workflow:apply_retention_plan(value)
	if type(value) ~= "table" then
		fail("retention plan must be an object")
	end
	local artifacts = value.manager and value.manager:prune(value.artifacts, true) or {}
	local worktrees = {}
	if value.worktrees and value.worktrees.manager and #value.worktrees.plan > 0 then
		worktrees = value.worktrees.manager:prune(value.worktrees.plan, true)
		for _, path in ipairs(worktrees) do
			local lease = value.by_path[path]
			if lease then
				self.store:remove_worktree_lease(lease.run_id)
			end
		end
	end
	local reclaimed_bytes = 0
	for _, value in ipairs(value.artifacts or {}) do
		reclaimed_bytes = reclaimed_bytes + (value.bytes or 0)
	end
	return { artifacts = artifacts, worktrees = worktrees, reclaimed_bytes = reclaimed_bytes }
end

function Workflow:startup_retention()
	if self.retention_started or not self.state.config.retention.cleanup_on_start then
		return { artifacts = {}, worktrees = {} }
	end
	self.retention_started = true
	local value = self:retention_plan({ quota = false })
	local ok, result = pcall(self.apply_retention_plan, self, value)
	if not ok then
		vim.notify("Gator startup cleanup: " .. tostring(result), vim.log.levels.WARN)
		return { artifacts = {}, worktrees = {} }
	end
	if #result.artifacts > 0 or #result.worktrees > 0 then
		vim.notify(
			"Gator startup cleanup: removed "
				.. #result.artifacts
				.. " artifacts and "
				.. #result.worktrees
				.. " clean worktrees",
			vim.log.levels.INFO
		)
	end
	return result
end

function Workflow:prune()
	local value = self:retention_plan()
	return self.retention_ui.open({
		artifacts = value.artifacts,
		worktrees = value.worktrees.plan,
		inventory = value.inventory,
		quota_bytes = value.quota_bytes,
		reclaim_bytes = value.reclaim_bytes,
		on_confirm = function()
			local ok, result = pcall(self.apply_retention_plan, self, value)
			vim.notify(
				ok
						and ("Gator cleanup: removed " .. #result.artifacts .. " artifacts · reclaimed " .. result.reclaimed_bytes .. " bytes · " .. #result.worktrees .. " worktrees")
					or ("Gator cleanup: " .. tostring(result)),
				ok and vim.log.levels.INFO or vim.log.levels.ERROR,
				{ title = "Gator" }
			)
		end,
	})
end

function Workflow:storage()
	local value = self:retention_plan()
	return self.retention_ui.open({
		artifacts = value.artifacts,
		worktrees = value.worktrees.plan,
		inventory = value.inventory,
		quota_bytes = value.quota_bytes,
		reclaim_bytes = value.reclaim_bytes,
		readonly = true,
	})
end

function Workflow:forget(id)
	local run = self:run(id)
	if active_state(run) then
		fail("cannot forget an active or detached run")
	end
	local lease = self.store:worktree_lease(run.id)
	if lease then
		if lease.state ~= "released" then
			fail("cannot forget a locked worktree lease")
		end
		local worktrees = self:worktree_cleanup_plan({ run_id = run.id, ignore_age = true })
		if #worktrees.plan ~= 1 then
			fail("cannot forget a worktree that is dirty, locked, missing, or not Gator-owned")
		end
		local removed = worktrees.manager:prune(worktrees.plan, true)
		if #removed ~= 1 then
			fail("cannot remove eligible worktree")
		end
		self.store:remove_worktree_lease(run.id)
	end
	return self.store:forget_run(run.id)
end

function Workflow:workspace(id, force_worktree)
	if not force_worktree then
		for _, run in ipairs(self:runs()) do
			if writers(run) then
				force_worktree = true
				break
			end
		end
	end
	if not force_worktree then
		return { kind = "project", root = self.root }
	end
	local parent = vim.fn.fnamemodify(self.root, ":h") .. "/gator-worktrees"
	if vim.fn.mkdir(parent, "p") ~= 1 and vim.fn.isdirectory(parent) ~= 1 then
		fail("cannot create Gator worktree parent")
	end
	local created = worktree.create({
		root = self.root,
		path = parent .. "/" .. id,
		branch = "gator/" .. id,
		base = "HEAD",
	})
	return { kind = "worktree", root = created.path, branch = created.branch, base = created.base }
end

function Workflow:handoff_workspace(source, force_worktree)
	if force_worktree then
		return self:workspace(run_store.id("handoff"), true)
	end
	return { kind = "project", root = self.root }
end

function Workflow:discard_workspace(workspace)
	if type(workspace) ~= "table" or workspace.kind ~= "worktree" then
		return false
	end
	local ok = pcall(worktree.remove, {
		root = self.root,
		path = workspace.root,
		branch = workspace.branch,
	})
	if not ok then
		vim.notify("Gator: retained unlaunched handoff worktree at " .. workspace.root, vim.log.levels.WARN)
		return false
	end
	return true
end

function Workflow:bundle(opts, workspace)
	local function finalize(body, estimate)
		local extras = self.extensions:collect({
			purpose = "launch",
			objective = opts.objective,
			workspace = workspace.root,
			capture = vim.deepcopy(opts.capture),
		})
		for _, artifact in ipairs(extras) do
			body = body .. "\n\n## Gator extension context · " .. artifact.name .. "\n" .. artifact.text
			estimate.artifacts = estimate.artifacts or {}
			table.insert(estimate.artifacts, {
				kind = "extension:" .. artifact.name,
				bytes = #artifact.text,
			})
		end
		local redacted, matches = self.extensions:redact(body)
		estimate.input_tokens = capture.estimate(redacted)
		estimate.redactions = (estimate.redactions or 0) + matches
		return redacted, estimate
	end
	if opts.bundle_body then
		local body = text(opts.bundle_body, "handoff bundle")
		return finalize(body, {
			input_tokens = capture.estimate(body),
			redactions = 0,
			artifacts = { { kind = "explicit_bundle", bytes = #body } },
		})
	end
	local diff, diff_redactions = capture.diff(workspace.root)
	local body, estimate = capture.bundle({
		objective = opts.objective,
		capture = opts.capture,
		profile = opts.profile or self.state.config.context.handoff.profile,
		max_chars = self.state.config.context.handoff.max_chars,
		diff = diff,
		diff_redactions = diff_redactions,
		note = opts.note,
		summary = opts.summary,
		transcript = opts.transcript,
	})
	return finalize(body, estimate)
end

function Workflow:open_conversation(run)
	local transcript = self:transcript(run)
	local history = transcript and vim.split(transcript, "\n", { plain = true, trimempty = false }) or {}
	conversation.open({
		provider = run.provider,
		session_id = run.session and run.session.id or run.id,
		run_id = run.id,
		state = run.state,
		history = history,
		on_input = function(message)
			self:send(run.id, message)
		end,
		on_cancel = function()
			self:cancel(run.id)
		end,
		on_detach = function()
			self:update(run.id, { state = "detached" })
		end,
		on_message = function() end,
	})
end

function Workflow:append_transcript(id, role, message)
	local run = self:run(id)
	if run.transcript ~= "available" or type(message) ~= "string" or message == "" then
		return false
	end
	self.transcripts[id] = self.transcripts[id] or {}
	table.insert(self.transcripts[id], "## " .. role .. "\n" .. message)
	self.store:transcript(id, table.concat(self.transcripts[id], "\n\n"))
	return true
end

function Workflow:close_loading(id)
	local handle = self.loading_handles[id]
	self.loading_handles[id] = nil
	if handle and type(handle.close) == "function" then
		pcall(handle.close)
	end
	return handle ~= nil
end

function Workflow:open_terminal(run, prepared)
	local definition = self:custom_provider(run.provider)
	local resume_supported = not definition or type(definition.resume) == "function"
	local opened = self.terminal:open({
		id = run.id,
		cwd = run.workspace.root,
		command = prepared.command,
		on_exit = function(result)
			self:close_loading(run.id)
			self:journal(run.id, "provider.exited", { code = result.code, transport = "terminal" })
			local latest = self.store:get(run.id)
			if latest and active_state(latest) then
				self:update(run.id, { state = result.code == 0 and "completed" or "failed" })
			end
			self.active[run.id] = nil
		end,
	})
	self.active[run.id] = { kind = "terminal", terminal_id = run.id }
	self:journal(run.id, "provider.session", {
		owner = "provider",
		resume_supported = resume_supported,
		transport = "terminal",
	})
	local updated = self:update(run.id, {
		state = "running",
		session = { id = prepared.session.id, resume_supported = resume_supported },
		process = { job_id = opened.job_id },
	})
	pcall(
		self.extensions.emit,
		self.extensions,
		"run.started",
		{ run_id = run.id, provider = run.provider, transport = "terminal" }
	)
	return updated
end

function Workflow:open_structured(run, prompt, operation, existing_session)
	local opened = self.structured:open({
		provider = run.provider,
		run_id = run.id,
		cwd = run.workspace.root,
		prompt = prompt,
		operation = operation,
		session = existing_session,
		codex_policy = run.provider == "codex" and trust.codex_policy(run.trust) or nil,
		on_session = function(session)
			self:close_loading(run.id)
			local state = prompt and "running" or "waiting_input"
			self:update(run.id, { session = session, state = state })
			self:journal(run.id, "provider.session", {
				owner = "provider",
				resume_supported = session.resume_supported,
				operation = operation or "start",
			})
			pcall(self.extensions.emit, self.extensions, "run.started", {
				run_id = run.id,
				provider = run.provider,
				transport = "chat",
			})
			pcall(conversation.update, { run_id = run.id, session_id = session.id, state = state })
		end,
		on_event = function(kind, value)
			if kind == "running" then
				self:journal(run.id, "provider.running", { transport = "structured" })
			elseif kind == "text" then
				self:append_transcript(run.id, "assistant", value)
				pcall(conversation.update, { run_id = run.id, text = value, state = "running" })
			elseif kind == "settled" then
				self:journal(run.id, "provider.settled", { transport = "structured" })
				self:update(run.id, { state = "waiting_input" })
				pcall(conversation.update, { run_id = run.id, state = "waiting_input" })
				self:finish_summary(run.id)
			elseif kind == "error" then
				self:journal(run.id, "provider.error", { code = failure_code(value), phase = "structured" })
				pcall(conversation.update, { run_id = run.id, text = value, state = "failed" })
			end
		end,
		on_usage = function(usage)
			self:report_usage(run.id, usage)
		end,
		on_approval = function(request, decide)
			local detail = request.command ~= "" and (" · " .. request.command) or ""
			pcall(
				conversation.update,
				{ run_id = run.id, text = "Approval requested: " .. request.action .. detail, state = "waiting_input" }
			)
			self:journal(run.id, "approval.requested", {
				action = request.action,
				has_command = request.command ~= "",
			})
			vim.ui.select({ "Approve once", "Deny", "Cancel" }, {
				prompt = "Gator approval · " .. run.provider .. " · " .. request.action .. detail,
			}, function(choice)
				local decision = choice == "Approve once" and "approved"
					or (choice == "Deny" and "denied" or "cancelled")
				decide(decision)
				self:journal(run.id, "approval.decided", { action = request.action, decision = decision })
			end)
		end,
		on_exit = function(result)
			self:close_loading(run.id)
			self:journal(run.id, "provider.exited", { code = result.code, transport = "structured" })
			local latest = self.store:get(run.id)
			if latest and active_state(latest) then
				self:update(run.id, { state = result.code == 0 and "completed" or "failed" })
			end
			self.active[run.id] = nil
		end,
	})
	self.active[run.id] = { kind = "structured", handle = opened }
	self:open_conversation(self:run(run.id))
	return run
end

function Workflow:open_managed(run, prompt, existing_session)
	local history
	if self.providers[run.provider].managed_mode == "history" then
		local directory = self.store.directory .. "/aider"
		vim.fn.mkdir(directory, "p")
		history = directory .. "/" .. run.id .. ".md"
	end
	local reference
	local function permission(request, respond)
		self:journal(run.id, "approval.requested", { action = request.action, provider = request.provider })
		vim.ui.select({ "approved", "denied", "cancelled" }, {
			prompt = "Gator approval · " .. request.provider .. " · " .. request.action,
		}, function(decision)
			local resolved = decision or "cancelled"
			respond(resolved)
			self:journal(run.id, "approval.decided", { action = request.action, decision = resolved })
		end)
	end
	local opened = self.managed:open({
		provider = run.provider,
		cwd = run.workspace.root,
		run_id = run.id,
		session = existing_session,
		history = history,
		prompt = prompt,
		on_session = function(session)
			self:close_loading(run.id)
			reference = session
			self.active[run.id] = { kind = "managed", reference = session }
			local state = prompt and "running" or "waiting_input"
			self:update(run.id, {
				state = state,
				session = {
					id = session.id,
					provider = session.provider,
					owner = session.owner,
					mode = session.mode,
					resume_supported = self.managed:can_resume(run.provider, session),
					capabilities = session.capabilities,
				},
			})
			self:journal(run.id, "provider.session", {
				owner = session.owner,
				mode = session.mode,
				resume_supported = self.managed:can_resume(run.provider, session),
			})
			pcall(self.extensions.emit, self.extensions, "run.started", {
				run_id = run.id,
				provider = run.provider,
				transport = "chat",
			})
			pcall(conversation.update, { run_id = run.id, session_id = session.id, state = state })
		end,
		on_event = function(event)
			if event.type == "text" or event.type == "complete" then
				self:append_transcript(run.id, "assistant", event.text or "")
				pcall(conversation.update, {
					run_id = run.id,
					text = event.text,
					state = event.type == "complete" and "waiting_input" or "running",
				})
				if event.type == "complete" then
					self:journal(run.id, "provider.settled", { transport = "managed" })
					self:update(run.id, { state = "waiting_input" })
					self:finish_summary(run.id)
				end
			elseif event.type == "error" then
				self:journal(run.id, "provider.error", { code = failure_code(event), phase = "managed" })
				pcall(conversation.update, { run_id = run.id, text = event.text, state = "failed" })
			end
		end,
		on_permission = permission,
		on_exit = function(result)
			self:close_loading(run.id)
			self:journal(run.id, "provider.exited", {
				code = result.code,
				stopped = result.stopped == true,
				transport = "managed",
			})
			local latest = self.store:get(run.id)
			if latest and active_state(latest) then
				self:update(
					run.id,
					{ state = result.stopped and "stopped" or (result.code == 0 and "completed" or "failed") }
				)
			end
			self.active[run.id] = nil
		end,
	})
	if reference then
		self.active[run.id] = { kind = "managed", reference = reference }
	end
	self:open_conversation(self:run(run.id))
	return opened
end

function Workflow:launch(opts)
	if type(opts) ~= "table" then
		fail("launch requires options")
	end
	local objective = text(opts.objective, "objective")
	local selected_role = role(opts.role)
	local chosen = self:provider(opts.provider)
	local transport = self:resolve_transport(chosen, opts.transport)
	local id = run_store.id("run")
	local workspace = opts.workspace or self:workspace(id, opts.force_worktree == true)
	if type(workspace) ~= "table" or (workspace.kind ~= "project" and workspace.kind ~= "worktree") then
		fail("workspace must identify a project or worktree")
	end
	if type(workspace.root) ~= "string" or not vim.uv.fs_realpath(workspace.root) then
		fail("workspace root must resolve")
	end
	local run_trust = trust.resolve({
		provider = chosen.provider,
		transport = transport,
		root = self.root,
		config = self.state.config,
	})
	if opts.native_session_operation ~= nil then
		if
			opts.native_session_operation ~= "fork"
			or transport ~= "chat"
			or not structured.supports(chosen.provider)
		then
			fail("native session operation is unavailable for this provider transport")
		end
		if type(opts.native_session) ~= "table" or type(opts.native_session.id) ~= "string" then
			fail("native session fork requires a provider session")
		end
	end
	local body, estimate = self:bundle(opts, workspace)
	local bundle_id = run_store.id("bundle")
	self.store:bundle(bundle_id, body)
	if opts.handoff_snapshot then
		self.store:materialize_handoff(bundle_id, body, opts.handoff_snapshot, workspace.root, {
			decisions = opts.handoff_decisions,
			apply = opts.apply_handoff_snapshot,
		})
	elseif workspace.kind == "worktree" then
		self.store:materialize_bundle(bundle_id, body, workspace.root)
	end
	local run = self:put({
		id = id,
		provider = chosen.provider,
		role = selected_role,
		transport = transport,
		state = "starting",
		workspace = workspace,
		parent_run_id = opts.parent_run_id,
		runbook_id = opts.runbook_id,
		depends_on = opts.depends_on,
		bundle_id = bundle_id,
		objective = objective,
		transcript = transport == "terminal" and "unavailable" or "available",
		usage = { state = "unknown", context_tokens_estimate = estimate.input_tokens },
		budget = {
			limit_tokens = self.state.config.budget.max_tokens,
			action = self.state.config.budget.action,
			state = self.state.config.budget.max_tokens > 0 and "unknown" or "unbounded",
		},
		resources = { started_at = self.clock(), context_bytes = 0, context_sends = 0 },
		trust = run_trust,
		created_at = self.clock(),
		updated_at = self.clock(),
	})
	local preflight = {
		purpose = "launch",
		provider = run.provider,
		transport = run.transport,
		bytes = #body,
		tokens = estimate.input_tokens,
		redactions = estimate.redactions or 0,
		artifacts = estimate.artifacts or {},
	}
	self:journal(
		run.id,
		"run.created",
		{ role = run.role, workspace = run.workspace.kind, parent_run_id = run.parent_run_id }
	)
	self:journal(run.id, "provider.selected", {
		provider = run.provider,
		transport = run.transport,
		readiness = chosen.readiness_state,
		version = chosen.version,
	})
	self:journal(run.id, "trust.applied", {
		surface = run.trust.surface,
		policy = run.trust.policy.state,
		policy_source = run.trust.policy.mode,
		write = run.trust.write.state,
		write_mode = run.trust.write.mode,
		network = run.trust.network.state,
		mcp = run.trust.mcp.state,
		approval = run.trust.approval.state,
	})
	self:journal(run.id, "context.prepared", preflight)
	local leased, lease_error = pcall(self.lease_worktree, self, run)
	if not leased then
		self:update(run.id, { state = "failed" })
		error(lease_error, 0)
	end
	local prompt = objective .. "\n\nUse this Gator context bundle:\n\n" .. body
	local instruction = role_instruction(selected_role)
	if instruction then
		prompt = prompt .. "\n\n" .. instruction
	end
	if opts.handoff_snapshot then
		local state = opts.apply_handoff_snapshot == false and "were retained" or "were applied and retained"
		prompt = prompt
			.. "\n\nReviewed source-file snapshots "
			.. state
			.. " at .gator/handoffs/"
			.. bundle_id
			.. "/files/."
	end
	local function begin()
		self:journal(run.id, "context.sent", preflight)
		self:record_context_delivery(run.id, #body)
		self.loading_handles[run.id] = self.loading.open({ message = "Starting " .. run.provider })
		vim.notify("Gator trust · " .. trust.summary(run.trust), vim.log.levels.INFO)
		if transport == "terminal" then
			self:prepare_terminal(run, prompt, function(prepared, reason)
				self:close_loading(run.id)
				if not prepared then
					self:journal(run.id, "provider.error", { code = failure_code(reason), phase = "launch" })
					self:update(run.id, { state = "failed" })
					vim.notify("Gator launch: " .. tostring(reason), vim.log.levels.ERROR)
					return
				end
				local ok, err = pcall(self.open_terminal, self, run, prepared)
				if not ok then
					self:journal(run.id, "provider.error", { code = failure_code(err), phase = "terminal_open" })
					self:update(run.id, { state = "failed" })
					vim.notify(tostring(err), vim.log.levels.ERROR, { title = "Gator" })
				end
			end)
		elseif structured.supports(run.provider) then
			local operation = opts.native_session_operation
			local existing = opts.native_session
			local ok, err = pcall(self.open_structured, self, run, prompt, operation, existing)
			if not ok then
				self:close_loading(run.id)
				self:journal(run.id, "provider.error", { code = failure_code(err), phase = "structured_open" })
				error(err, 0)
			end
		else
			local ok, err = pcall(self.open_managed, self, run, prompt)
			if not ok then
				self:close_loading(run.id)
				self:journal(run.id, "provider.error", { code = failure_code(err), phase = "managed_open" })
				error(err, 0)
			end
		end
		if opts.remember ~= false then
			self.store:set_default_provider(run.provider)
		end
	end
	if self.state.config.context.preflight.confirm then
		self:open_preflight(preflight, begin, function()
			self:journal(run.id, "context.cancelled", { purpose = "launch" })
			self:update(run.id, { state = "stopped" })
		end)
	else
		begin()
	end
	return run
end

function Workflow:send(id, message)
	message = text(message, "prompt")
	local run = self:run(id)
	local active = self.active[id]
	if not active then
		fail("run is not active in this Neovim instance")
	end
	if active.kind == "structured" then
		if not active.handle.send(message) then
			fail("structured provider rejected the prompt")
		end
	elseif active.kind == "managed" then
		if not active.reference then
			fail("provider session is not ready")
		end
		self.managed:send(active.reference, message)
	else
		fail("native terminal runs accept input in their terminal")
	end
	self:append_transcript(id, "user", message)
	self:update(run.id, { state = "running" })
	return true
end

function Workflow:send_context(opts)
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("send context requires options")
	end
	local run_id = opts.run_id
	if run_id == nil then
		local choices = {}
		for _, candidate in ipairs(self:runs()) do
			local active = self.active[candidate.id]
			if
				candidate.transport == "chat"
				and active
				and (active.kind == "structured" or active.kind == "managed")
			then
				table.insert(choices, candidate)
			end
		end
		if #choices == 0 then
			fail("no active structured Gator chat can receive editor context")
		end
		vim.ui.select(choices, {
			prompt = "Send editor context to Gator run",
			format_item = function(value)
				return value.id .. " · " .. value.provider .. " · " .. value.objective
			end,
		}, function(choice)
			if choice then
				opts.run_id = choice.id
				local ok, err = pcall(self.send_context, self, opts)
				if not ok then
					vim.notify(tostring(err), vim.log.levels.ERROR, { title = "Gator" })
				end
			end
		end)
		return true
	end
	local run = self:run(run_id)
	local active = self.active[run.id]
	if run.transport ~= "chat" or not active or (active.kind ~= "structured" and active.kind ~= "managed") then
		fail("editor context can be sent only to an active structured Gator chat")
	end
	if opts.kind == nil then
		vim.ui.select(
			{ "selection", "diagnostic", "hunk", "bundle" },
			{ prompt = "Gator context kind" },
			function(choice)
				if choice then
					opts.kind = choice
					local ok, err = pcall(self.send_context, self, opts)
					if not ok then
						vim.notify(tostring(err), vim.log.levels.ERROR, { title = "Gator" })
					end
				end
			end
		)
		return true
	end
	local kind = context_kind(opts.kind)
	if kind == "bundle" and opts.bundle_id == nil then
		vim.ui.input({ prompt = "Gator context bundle id: " }, function(value)
			if type(value) == "string" and vim.trim(value) ~= "" then
				opts.bundle_id = value
				local ok, err = pcall(self.send_context, self, opts)
				if not ok then
					vim.notify(tostring(err), vim.log.levels.ERROR, { title = "Gator" })
				end
			end
		end)
		return true
	end
	local message
	local artifacts
	local redactions = 0
	if kind == "selection" then
		local selected = capture.current(opts)
		artifacts = {
			{
				kind = "selection",
				path = selected.path,
				first_line = selected.first_line,
				last_line = selected.last_line,
				bytes = #selected.text,
			},
		}
		redactions = selected.redactions or 0
		message = table.concat({
			"## Gator follow-up editor context",
			"- Kind: selection",
			"- Path: `" .. selected.path .. "`",
			"- Lines: " .. selected.first_line .. "-" .. selected.last_line,
			"- Changedtick: " .. selected.changedtick,
			"",
			"```" .. selected.language,
			selected.text,
			"```",
		}, "\n")
	elseif kind == "diagnostic" then
		local diagnostics = capture.diagnostics(opts)
		artifacts = {
			{
				kind = "diagnostics",
				path = diagnostics.path,
				first_line = diagnostics.first_line,
				last_line = diagnostics.last_line,
				count = #diagnostics.values,
				bytes = 0,
			},
		}
		redactions = diagnostics.redactions or 0
		local lines = {
			"## Gator follow-up editor context",
			"- Kind: diagnostic",
			"- Path: `" .. diagnostics.path .. "`",
			"- Lines: " .. diagnostics.first_line .. "-" .. diagnostics.last_line,
			"",
		}
		for _, item in ipairs(diagnostics.values) do
			table.insert(
				lines,
				"- " .. item.severity .. " " .. item.line .. ":" .. item.column .. " · " .. item.message
			)
		end
		message = table.concat(lines, "\n")
		artifacts[1].bytes = #message
	elseif kind == "hunk" then
		local hunk = capture.hunk({ root = run.workspace.root, buffer = opts.buffer })
		artifacts = { { kind = "git_hunk", path = hunk.path, bytes = #hunk.text } }
		redactions = hunk.redactions or 0
		message = table.concat({
			"## Gator follow-up editor context",
			"- Kind: Git hunk",
			"- Path: `" .. hunk.path .. "`",
			"",
			"```diff",
			hunk.text,
			"```",
		}, "\n")
	else
		local bundle = self.store:read_bundle(opts.bundle_id)
		if not bundle then
			fail("context bundle is unavailable: " .. tostring(opts.bundle_id))
		end
		message = "## Gator follow-up editor context\n- Kind: named bundle\n- Bundle: `"
			.. opts.bundle_id
			.. "`\n\n"
			.. bundle
		artifacts = { { kind = "named_bundle", bundle_id = opts.bundle_id, bytes = #bundle } }
	end
	for _, artifact in
		ipairs(self.extensions:collect({
			purpose = "send",
			run_id = run.id,
			provider = run.provider,
			kind = kind,
			workspace = run.workspace.root,
		}))
	do
		message = message .. "\n\n## Gator extension context · " .. artifact.name .. "\n" .. artifact.text
		table.insert(artifacts, { kind = "extension:" .. artifact.name, bytes = #artifact.text })
	end
	local redacted, extension_redactions = self.extensions:redact(message)
	message = redacted
	redactions = redactions + extension_redactions
	local preflight = {
		purpose = "send",
		provider = run.provider,
		transport = run.transport,
		bytes = #message,
		tokens = capture.estimate(message),
		redactions = redactions,
		artifacts = artifacts,
	}
	self:journal(run.id, "context.prepared", preflight)
	local function deliver()
		self:journal(run.id, "context.sent", preflight)
		self:record_context_delivery(run.id, #message)
		return self:send(run.id, message)
	end
	if self.state.config.context.preflight.confirm then
		self:open_preflight(preflight, deliver, function()
			self:journal(run.id, "context.cancelled", { purpose = "send" })
		end)
		return true
	end
	return deliver()
end

function Workflow:attach_context(id)
	return self:send_context({ run_id = id })
end

function Workflow:review_context(run)
	local base_sha = capture.head(run.workspace.root)
	if not base_sha then
		fail("review requires a Git workspace with HEAD")
	end
	local diff = capture.diff(run.workspace.root) or ""
	return { base_sha = base_sha, diff = diff, diff_sha256 = vim.fn.sha256(diff) }
end

function Workflow:review(id)
	if id == nil then
		local runs = self:runs()
		if #runs == 0 then
			fail("no Gator runs are available for review")
		end
		vim.ui.select(runs, {
			prompt = "Review Gator run",
			format_item = function(value)
				return value.id .. " · " .. value.provider .. " · " .. value.role .. " · " .. value.state
			end,
		}, function(choice)
			if choice then
				local ok, err = pcall(self.review, self, choice.id)
				if not ok then
					vim.notify(tostring(err), vim.log.levels.ERROR, { title = "Gator" })
				end
			end
		end)
		return true
	end
	local run = self:run(id)
	local context = self:review_context(run)
	local commands = vim.tbl_keys(self.state.config.review.commands)
	table.sort(commands)
	self.review_sessions[run.id] = context
	return self.review_ui.open({
		run = run,
		workspace = run.workspace.root,
		base_sha = context.base_sha,
		diff_sha256 = context.diff_sha256,
		diff = context.diff,
		commands = commands,
		on_test = function(command_id)
			vim.ui.select({ "Run approved test", "Cancel" }, {
				prompt = "Gator review · run " .. command_id .. " in " .. run.workspace.root,
			}, function(choice)
				if choice == "Run approved test" then
					local ok, err = pcall(self.execute_review_test, self, run.id, command_id, true)
					if not ok then
						vim.notify(tostring(err), vim.log.levels.ERROR, { title = "Gator" })
					end
				end
			end)
		end,
		on_decision = function(decision)
			local ok, err = pcall(self.record_review_decision, self, run.id, decision)
			if not ok then
				vim.notify(tostring(err), vim.log.levels.ERROR, { title = "Gator" })
				return
			end
			self.review_ui.close()
			if decision == "handoff" then
				self:handoff(run.id)
			end
		end,
	})
end

function Workflow:review_policy(run)
	return policy_overlay.new({
		scope = "run",
		target = run.id,
		rules = { test_commands = vim.deepcopy(self.state.config.review.commands) },
		provenance = { source = "gator.setup", ref = "review.commands" },
	})
end

function Workflow:execute_review_test(id, command_id, confirmed)
	if confirmed ~= true then
		fail("review test execution requires explicit confirmation")
	end
	local run = self:run(id)
	local context = self.review_sessions[run.id]
	if not context then
		fail("open a Gator review before running its approved test")
	end
	local current = self:review_context(run)
	if current.base_sha ~= context.base_sha or current.diff_sha256 ~= context.diff_sha256 then
		fail("review is stale because the target worktree changed")
	end
	local output = {}
	local result = self.review_validation.execute({
		policy = self:review_policy(run),
		command_id = command_id,
		workspace = run.workspace,
		on_evidence = function(value)
			table.insert(output, "[" .. value.stream .. "]\n" .. value.text)
		end,
	})
	local record = self.store:write_review({
		id = run_store.id("review"),
		run_id = run.id,
		state = result.passed and "passed" or "failed",
		base_sha = context.base_sha,
		diff_sha256 = context.diff_sha256,
		command_id = result.command_id,
		argv = result.argv,
		exit_code = result.code,
		passed = result.passed,
		created_at = now(),
	}, table.concat(output, "\n"))
	self:update(run.id, { review = { id = record.review.id, state = record.review.state } })
	self:journal(run.id, "review.recorded", {
		review_id = record.review.id,
		state = record.review.state,
		command_id = record.review.command_id,
		passed = record.review.passed,
	})
	vim.notify(
		"Gator review test "
			.. result.command_id
			.. " "
			.. (result.passed and "passed" or "failed")
			.. " · evidence "
			.. record.output_ref,
		result.passed and vim.log.levels.INFO or vim.log.levels.WARN
	)
	return record
end

function Workflow:record_review_decision(id, decision)
	if decision ~= "accepted" and decision ~= "changes_requested" and decision ~= "handoff" then
		fail("review decision is unavailable")
	end
	local run = self:run(id)
	local context = self.review_sessions[run.id] or self:review_context(run)
	local current = self:review_context(run)
	if current.base_sha ~= context.base_sha or current.diff_sha256 ~= context.diff_sha256 then
		fail("review is stale because the target worktree changed")
	end
	if decision == "accepted" and next(self.state.config.review.commands) ~= nil then
		local passed = false
		for _, evidence in ipairs(self.store:list_reviews(run.id)) do
			if
				evidence.state == "passed"
				and evidence.base_sha == context.base_sha
				and evidence.diff_sha256 == context.diff_sha256
			then
				passed = true
				break
			end
		end
		if not passed then
			fail("accepted review requires passed evidence for the reviewed worktree diff")
		end
	end
	local record = self.store:write_review({
		id = run_store.id("review"),
		run_id = run.id,
		state = decision,
		base_sha = context.base_sha,
		diff_sha256 = context.diff_sha256,
		decision = decision,
		created_at = now(),
	}, "Review decision: " .. decision)
	self:update(run.id, { review = { id = record.review.id, state = record.review.state } })
	self:journal(
		run.id,
		"review.recorded",
		{ review_id = record.review.id, state = record.review.state, decision = decision }
	)
	return record
end

function Workflow:create_runbook(opts)
	if type(opts) ~= "table" then
		fail("create runbook requires options")
	end
	local record = vim.deepcopy(opts)
	record.id = record.id or run_store.id("runbook")
	record.created_at = record.created_at or now()
	record.max_concurrent = record.max_concurrent or self.state.config.runbooks.max_concurrent
	record.max_tokens = record.max_tokens or self.state.config.runbooks.max_tokens
	for _, step in ipairs(record.steps or {}) do
		if step.provider == nil then
			fail("runbook steps require an explicit provider")
		end
	end
	return self.store:create_runbook(record)
end

function Workflow:runbooks()
	return self.store:list_runbooks()
end

function Workflow:runbook_status(id)
	local runbook = self.store:runbook(id)
	if not runbook then
		fail("runbook is unavailable: " .. tostring(id))
	end
	local runs = {}
	for _, run in ipairs(self:runs()) do
		runs[run.id] = run
	end
	local steps, by_id, reported_tokens, unknown_usage, tracked_runs, active = {}, {}, 0, false, 0, 0
	for _, run in pairs(runs) do
		if run.runbook_id == runbook.id then
			tracked_runs = tracked_runs + 1
			if active_state(run) then
				active = active + 1
			end
			if run.usage.state == "reported" and type(run.usage.total_tokens) == "number" then
				reported_tokens = reported_tokens + run.usage.total_tokens
			else
				unknown_usage = true
			end
		end
	end
	for _, step in ipairs(runbook.steps) do
		local run = step.run_id and runs[step.run_id] or nil
		local state = run and run.state or "pending"
		by_id[step.id] = { step = step, run = run, state = state }
	end
	for _, step in ipairs(runbook.steps) do
		local status = by_id[step.id]
		local dependencies_complete = true
		for _, dependency in ipairs(step.depends_on) do
			local source = by_id[dependency]
			if not source.run or source.run.state ~= "completed" then
				dependencies_complete = false
				break
			end
		end
		status.ready = status.run == nil and dependencies_complete
		table.insert(steps, {
			id = step.id,
			role = step.role,
			provider = step.provider,
			objective = step.objective,
			depends_on = vim.deepcopy(step.depends_on),
			run_id = step.run_id,
			state = status.state,
			ready = status.ready,
		})
	end
	return {
		id = runbook.id,
		title = runbook.title,
		max_concurrent = runbook.max_concurrent,
		max_tokens = runbook.max_tokens,
		reported_tokens = reported_tokens,
		usage_state = tracked_runs == 0 and "unknown" or (unknown_usage and "partial" or "reported"),
		tracked_runs = tracked_runs,
		active = active,
		steps = steps,
	}
end

function Workflow:ready_runbook_steps()
	local result = {}
	for _, runbook in ipairs(self:runbooks()) do
		local status = self:runbook_status(runbook.id)
		for _, step in ipairs(status.steps) do
			if step.ready then
				table.insert(result, { runbook_id = status.id, title = status.title, step = step })
			end
		end
	end
	table.sort(result, function(left, right)
		return left.runbook_id == right.runbook_id and left.step.id < right.step.id
			or left.runbook_id < right.runbook_id
	end)
	return result
end

function Workflow:check_runbook_limits(runbook_id)
	local global_limit = self.state.config.budget.max_concurrent_runs
	local globally_active = 0
	for _, run in ipairs(self:runs()) do
		if active_state(run) then
			globally_active = globally_active + 1
		end
	end
	if global_limit > 0 and globally_active >= global_limit then
		fail("global Gator concurrency limit is reached")
	end
	local status = self:runbook_status(runbook_id)
	local configured = self.state.config.runbooks.max_concurrent
	local limit = status.max_concurrent > 0 and status.max_concurrent or configured
	if limit > 0 and status.active >= limit then
		fail("runbook concurrency limit is reached")
	end
	local budget = status.max_tokens > 0 and status.max_tokens or self.state.config.runbooks.max_tokens
	if budget > 0 and status.reported_tokens >= budget then
		fail("runbook reported-token budget is exhausted")
	end
	return status
end

function Workflow:bind_runbook_step(runbook_id, step_id, run)
	local record = self.store:runbook(runbook_id)
	if not record then
		fail("runbook is unavailable")
	end
	for _, step in ipairs(record.steps) do
		if step.id == step_id then
			step.run_id = run.id
			self.store:update_runbook(record)
			return run
		end
	end
	fail("runbook step is unavailable")
end

function Workflow:start_runbook_step(runbook_id, step_id, confirmed)
	local status = self:check_runbook_limits(runbook_id)
	local step
	for _, value in ipairs(status.steps) do
		if value.id == step_id then
			step = value
			break
		end
	end
	if not step or not step.ready then
		fail("runbook step is not ready")
	end
	if step.role == "integrator" and confirmed ~= true then
		fail("integrator runbook steps require explicit user confirmation")
	end
	local dependencies = {}
	for _, dependency in ipairs(step.depends_on) do
		for _, candidate in ipairs(status.steps) do
			if candidate.id == dependency and candidate.run_id then
				table.insert(dependencies, candidate.run_id)
			end
		end
	end
	local function bound(run)
		return self:bind_runbook_step(runbook_id, step.id, run)
	end
	local dependency_context = self:runbook_dependency_context(dependencies)
	if step.role == "reviewer" and #dependencies > 0 then
		local source = self:run(dependencies[#dependencies])
		return self:handoff(source.id, step.provider, {
			profile = "full",
			role = step.role,
			objective = step.objective,
			runbook_id = runbook_id,
			depends_on = dependencies,
			additional_context = dependency_context,
			reuse_source_workspace = true,
			on_launch = bound,
		})
	end
	local workspace = nil
	if step.role == "researcher" then
		workspace = { kind = "project", root = self.root }
	end
	local run = self:launch({
		objective = step.objective,
		provider = step.provider,
		role = step.role,
		workspace = workspace,
		force_worktree = step.role == "writer" or step.role == "integrator",
		runbook_id = runbook_id,
		depends_on = dependencies,
		bundle_body = dependency_context,
	})
	return bound(run)
end

function Workflow:start_ready_runbook_step()
	local choices = self:ready_runbook_steps()
	if #choices == 0 then
		fail("no runbook step is ready")
	end
	vim.ui.select(choices, {
		prompt = "Start ready Gator runbook step",
		format_item = function(value)
			return value.runbook_id
				.. " · "
				.. value.step.id
				.. " · "
				.. value.step.role
				.. " · "
				.. value.step.objective
		end,
	}, function(choice)
		if not choice then
			return
		end
		local function start(confirmed)
			local ok, err = pcall(self.start_runbook_step, self, choice.runbook_id, choice.step.id, confirmed)
			if not ok then
				vim.notify(tostring(err), vim.log.levels.ERROR, { title = "Gator" })
			end
		end
		if choice.step.role == "integrator" then
			vim.ui.select(
				{ "Start integrator", "Cancel" },
				{ prompt = "Gator integrator convergence approval" },
				function(value)
					if value == "Start integrator" then
						start(true)
					end
				end
			)
		else
			start(false)
		end
	end)
	return true
end

function Workflow:cancel(id)
	local active = self.active[id]
	if not active then
		return false
	end
	if active.kind == "structured" then
		return active.handle.cancel()
	elseif active.kind == "managed" then
		return self.managed:cancel(active.reference)
	end
	return false
end

function Workflow:stop(id)
	local run = self:run(id)
	self:close_loading(id)
	local active = self.active[id]
	if active then
		if active.kind == "terminal" then
			self.terminal:close(active.terminal_id)
		elseif active.kind == "structured" then
			active.handle.stop()
		elseif active.kind == "managed" and active.reference then
			self.managed:stop(active.reference)
		end
		self.active[id] = nil
	end
	self:journal(run.id, "provider.exited", { stopped = true, transport = run.transport })
	self:update(run.id, { state = "stopped" })
	return true
end

function Workflow:focus(id)
	local run = self:run(id)
	local active = self.active[id]
	if active and active.kind == "terminal" then
		return self.terminal:attach(active.terminal_id)
	end
	if run.transport == "chat" then
		if self.active[id] then
			return self:open_conversation(run)
		end
		return self:resume(id)
	end
	return self:resume(id)
end

function Workflow:resume(id)
	local run = self:run(id)
	if not run.session or not run.session.resume_supported then
		fail("this run cannot be resumed through Gator")
	end
	if run.transport == "chat" then
		self:update(id, { state = "starting" })
		self.loading_handles[run.id] = self.loading.open({ message = "Resuming " .. run.provider })
		if structured.supports(run.provider) then
			return self:open_structured(run, nil, "resume", run.session)
		end
		return self:open_managed(run, nil, run.session)
	end
	local ok = pcall(self.terminal.attach, self.terminal, id)
	if ok then
		return true
	end
	self:prepare_terminal(run, nil, function(prepared, reason)
		if not prepared then
			self:journal(run.id, "provider.error", { code = failure_code(reason), phase = "resume" })
			vim.notify("Gator resume: " .. tostring(reason), vim.log.levels.ERROR)
			return
		end
		self:open_terminal(run, prepared)
	end, true)
	return true
end

function Workflow:fork(id)
	local source = self:run(id)
	if source.transport ~= "chat" or not source.session or not source.session.resume_supported then
		fail("this run cannot be forked through Gator")
	end
	if not structured.supports(source.provider) then
		fail(source.provider .. " does not expose a documented native chat fork contract")
	end
	return self:handoff(source.id, source.provider, { profile = "full", native_fork = true })
end

function Workflow:transcript(run)
	local value = self.transcripts[run.id]
	return value and table.concat(value, "\n\n") or self.store:read_transcript(run.id)
end

function Workflow:runbook_dependency_context(ids)
	if type(ids) ~= "table" or not vim.islist(ids) then
		fail("runbook dependency ids must be an array")
	end
	local lines, dependencies =
		{
			"## Gator runbook dependency context",
			"This is Gator-owned provenance. Provider-native sessions were not migrated.",
		}, {}
	for _, id in ipairs(ids) do
		local dependency = self:run(id)
		table.insert(dependencies, dependency)
		table.insert(
			lines,
			"- `"
				.. dependency.id
				.. "` · "
				.. dependency.role
				.. " · "
				.. dependency.provider
				.. " · "
				.. dependency.state
		)
	end
	local maximum = self.state.config.context.handoff.max_chars
	local header = table.concat(lines, "\n")
	if #header > maximum then
		fail("configured handoff max_chars is too small for runbook dependency provenance")
	end
	for _, dependency in ipairs(dependencies) do
		vim.list_extend(lines, {
			"",
			"### " .. dependency.id .. " · " .. dependency.role .. " · " .. dependency.provider,
			"- State: " .. dependency.state,
			"- Workspace: `" .. dependency.workspace.root .. "`",
			"- Objective: " .. dependency.objective,
		})
		local bundle = dependency.bundle_id and self.store:read_bundle(dependency.bundle_id) or nil
		if bundle then
			vim.list_extend(lines, { "", "#### Original context bundle", bundle })
		end
		local transcript = self:transcript(dependency)
		if transcript then
			vim.list_extend(lines, { "", "#### Gator-owned transcript", transcript })
		end
		local diff = capture.diff(dependency.workspace.root)
		if diff and diff ~= "" then
			vim.list_extend(lines, { "", "#### Current dependency diff", "```diff", diff, "```" })
		end
	end
	local body = table.concat(lines, "\n")
	if #body > maximum then
		local suffix = "\n\n[Gator dependency detail truncated at configured handoff bound.]"
		if #header + #suffix > maximum then
			fail("configured handoff max_chars is too small for runbook dependency provenance")
		end
		body = body:sub(1, maximum - #suffix) .. suffix
	end
	return body
end

function Workflow:finish_summary(id)
	local pending = self.pending_summary[id]
	if not pending then
		return false
	end
	self.pending_summary[id] = nil
	local parts = self.transcripts[id] or {}
	local summary = table.concat(vim.list_slice(parts, pending.start_index + 1), "\n\n")
	if vim.trim(summary) == "" then
		vim.notify("Gator handoff: source agent returned no summary", vim.log.levels.WARN)
		return false
	end
	return self:handoff(id, pending.target, { profile = "compact", summary = summary })
end

function Workflow:handoff(source_id, target, opts)
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("handoff options must be an object")
	end
	if opts.on_launch ~= nil and type(opts.on_launch) ~= "function" then
		fail("handoff on_launch must be a function")
	end
	local source = self:run(source_id)
	if not target then
		local choices = {}
		for _, value in pairs(self.providers) do
			if opts.include_source or value.provider ~= source.provider then
				table.insert(choices, value)
			end
		end
		return self:pick_provider(choices, function(choice)
			self:handoff(source_id, choice.provider, opts)
		end)
	end
	target = self:provider(target).provider
	local profile = opts.profile or self.state.config.context.handoff.profile
	if profile == "summary-first" and not self.state.config.context.handoff.source_summary then
		fail("summary-first handoff requires context.handoff.source_summary = true")
	end
	if profile == "summary-first" and source.transcript == "unavailable" then
		fail("summary-first handoff is unavailable for terminal-originated runs")
	end
	local body
	local bundle_path = self.store.bundles_directory .. "/" .. source.bundle_id .. ".md"
	body = vim.fn.filereadable(bundle_path) == 1 and table.concat(vim.fn.readfile(bundle_path), "\n")
		or source.objective
	if type(opts.summary) == "string" and vim.trim(opts.summary) ~= "" then
		body = body .. "\n\n## Source-agent summary\n" .. opts.summary
	end
	if type(opts.additional_context) == "string" and vim.trim(opts.additional_context) ~= "" then
		body = body .. "\n\n" .. opts.additional_context
	end
	for _, section in
		ipairs(self.extensions:format_handoff({
			source = vim.deepcopy(source),
			target = target,
			profile = profile,
			body = body,
		}))
	do
		body = body .. "\n\n## Gator extension handoff · " .. section.name .. "\n" .. section.text
	end
	body = self.extensions:redact(body)
	local snapshot = capture.snapshot(source.workspace.root, {
		max_files = self.state.config.context.handoff.max_files,
		max_file_chars = self.state.config.context.handoff.max_file_chars,
	})
	body = body .. "\n\n" .. capture.snapshot_markdown(snapshot)
	if profile == "compact" then
		body = body:sub(1, self.state.config.context.handoff.max_chars)
	elseif profile == "full" then
		local transcript = self:transcript(source)
		if transcript then
			body = body .. "\n\n## Gator-owned transcript\n" .. transcript
		end
	elseif profile == "summary-first" then
		local active = self.active[source.id]
		if not active or source.transport ~= "chat" then
			fail("summary-first handoff requires an active Gator chat source")
		end
		vim.ui.select({ "Ask source agent", "Cancel" }, { prompt = "Gator handoff summary" }, function(choice)
			if choice ~= "Ask source agent" then
				return
			end
			self.pending_summary[source.id] = { target = target, start_index = #(self.transcripts[source.id] or {}) }
			local ok, err = pcall(
				self.send,
				self,
				source.id,
				"Prepare a concise handoff summary: current result, changes made, unresolved risks, and recommended next action."
			)
			if not ok then
				self.pending_summary[source.id] = nil
				vim.notify("Gator handoff: " .. tostring(err), vim.log.levels.ERROR)
			end
		end)
		return true
	end
	body = self.extensions:redact(body)
	local owned_workspace = false
	local workspace
	if opts.workspace then
		workspace = vim.deepcopy(opts.workspace)
	elseif opts.reuse_source_workspace then
		workspace = vim.deepcopy(source.workspace)
	else
		workspace =
			self:handoff_workspace(source, opts.force_worktree == true or opts.native_fork == true or writers(source))
		owned_workspace = workspace.kind == "worktree"
	end
	local apply_snapshot = workspace.root ~= source.workspace.root
	local conflicts = apply_snapshot and self.store:handoff_conflicts(snapshot, workspace.root) or {}
	local preview = apply_snapshot and self.store:preview_handoff(snapshot, workspace.root)
		or "Target is the source workspace; snapshots will be retained as an artifact but not applied again."
	local included, omitted = 0, 0
	for _, file in ipairs(snapshot.files) do
		if file.state == "included" or file.state == "deleted" then
			included = included + 1
		else
			omitted = omitted + 1
		end
	end
	self:journal(source.id, "handoff.prepared", {
		target = target,
		profile = profile,
		bytes = #body,
		included = included,
		omitted = omitted,
	})
	local review_options = {
		source = source,
		target = target,
		profile = profile,
		body = body,
		preflight = {
			transport = self:resolve_transport(self:provider(target)),
			workspace = vim.fn.fnamemodify(workspace.root, ":~:."),
			context_bytes = #body,
			included = included,
			omitted = omitted,
		},
		apply_snapshot = apply_snapshot,
		preview = preview,
		conflicts = conflicts,
		on_cancel = function()
			if owned_workspace then
				self:discard_workspace(workspace)
			end
		end,
		on_confirm = function(reviewed, decisions, approved_snapshot_application)
			self:journal(source.id, "handoff.reviewed", { target = target, profile = profile })
			local materialize_snapshot = approved_snapshot_application
			if materialize_snapshot == nil then
				materialize_snapshot = apply_snapshot
			end
			local launched, run = pcall(self.launch, self, {
				objective = opts.objective or ("Continue the reviewed handoff from " .. source.provider),
				provider = target,
				bundle_body = reviewed,
				parent_run_id = source.id,
				role = opts.role or "writer",
				workspace = workspace,
				remember = false,
				handoff_snapshot = snapshot,
				handoff_decisions = decisions,
				apply_handoff_snapshot = materialize_snapshot,
				native_session_operation = opts.native_fork and "fork" or nil,
				native_session = opts.native_fork and source.session or nil,
				runbook_id = opts.runbook_id,
				depends_on = opts.depends_on,
			})
			if not launched then
				if owned_workspace then
					self:discard_workspace(workspace)
				end
				vim.notify(tostring(run), vim.log.levels.ERROR, { title = "Gator" })
				return
			end
			self:journal(source.id, "handoff.delivered", { target = target, run_id = run.id })
			if opts.on_launch then
				local bound, bind_err = pcall(opts.on_launch, run)
				if not bound then
					vim.notify("Gator runbook binding: " .. tostring(bind_err), vim.log.levels.ERROR)
				end
			end
		end,
	}
	return self:render_ui("handoff_review", {
		source = vim.deepcopy(source),
		target = target,
		profile = profile,
		body = body,
		preflight = vim.deepcopy(review_options.preflight),
		conflicts = vim.deepcopy(conflicts),
		actions = { confirm = review_options.on_confirm, cancel = review_options.on_cancel },
	}, function()
		return self.handoff_review.open(review_options)
	end)
end

function Workflow:launch_parallel(id)
	local source = self:run(id)
	return self:handoff(source.id, nil, { profile = "compact", include_source = true })
end

function Workflow:open_runs()
	return self:render_ui("run_graph", {
		runs = self:runs(),
		columns = self:graph_columns(),
		actions = {
			focus = function(id)
				return self:focus(id)
			end,
			stop = function(id)
				return self:stop(id)
			end,
			handoff = function(id, target)
				return self:handoff(id, target)
			end,
			context = function(id)
				return self:attach_context(id)
			end,
		},
	}, function()
		return run_graph.open(self)
	end)
end

function Workflow:close()
	for id in pairs(vim.deepcopy(self.loading_handles)) do
		self:close_loading(id)
	end
	for id in pairs(vim.deepcopy(self.active)) do
		pcall(self.stop, self, id)
	end
	self.managed:shutdown()
	return true
end

return M
