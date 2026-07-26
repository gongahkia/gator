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
local worktree = require("gator.workspace.worktree")
local state_store = require("gator.state")

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

local function active_state(value)
	return value.state == "starting" or value.state == "running" or value.state == "waiting_input" or value.state == "detached"
end

local function writers(value)
	return active_state(value) and (value.role == "primary" or value.role == "writer")
end

function M.new(opts)
	opts = opts or {}
	if type(opts) ~= "table" or not state_store.is(opts.state) then
		fail("new requires initialized state")
	end
	for key in pairs(opts) do
		if key ~= "state" and key ~= "root" and key ~= "terminal" and key ~= "bridge" and key ~= "readiness" and key ~= "managed" and key ~= "structured" then
			fail("new contains unsupported field: " .. tostring(key))
		end
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
		providers = {},
		active = {},
		transcripts = {},
	}, Workflow)
	value:refresh()
	return value
end

function Workflow:runs()
	return self.store:list()
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

function Workflow:update(id, patch)
	local value = self:run(id)
	for key, item in pairs(patch) do
		value[key] = vim.deepcopy(item)
	end
	value.updated_at = now()
	return self:put(value)
end

function Workflow:refresh()
	local providers = {}
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
			}
		end
	end
	local confirmations = {}
	for name, value in pairs(self.state.config.providers) do
		confirmations[name] = value.user_confirmed
	end
	for _, record in ipairs(managed_adapter.catalog({ cwd = self.root, user_confirmed = confirmations })) do
		if record.available then
			providers[record.provider] = {
				provider = record.provider,
				available = true,
				chat = true,
				terminal = native_terminal.supports(record.provider),
				managed_mode = record.mode,
				readiness_state = record.readiness_state,
			}
		end
	end
	self.providers = providers
	self.state:update({ adapters = vim.deepcopy(providers) })
	return vim.deepcopy(providers)
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
	self:refresh()
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
	return provider_picker.open({
		providers = choices,
		on_launch = function(choice)
			opts.provider = choice.provider
			local ok, err = pcall(self.launch, self, opts)
			if not ok then
				vim.notify(tostring(err), vim.log.levels.ERROR, { title = "Gator" })
			end
		end,
	})
end

function Workflow:prompt(opts)
	opts = opts or {}
	local selected = capture.current({ buffer = opts.buffer, first_line = opts.first_line, last_line = opts.last_line })
	vim.ui.input({ prompt = "Gator: " }, function(objective)
		if type(objective) == "string" and vim.trim(objective) ~= "" then
			local ok, err = pcall(self.choose, self, {
				objective = objective,
				capture = selected,
				provider = opts.provider,
				transport = opts.transport,
			})
			if not ok then
				vim.notify(tostring(err), vim.log.levels.ERROR, { title = "Gator" })
			end
		end
	end)
	return true
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
	return { kind = "worktree", root = created.path, branch = created.branch }
end

function Workflow:bundle(opts, workspace)
	if opts.bundle_body then
		local body = text(opts.bundle_body, "handoff bundle")
		return body, { state = "estimated", input_tokens = capture.estimate(body) }
	end
	return capture.bundle({
		objective = opts.objective,
		capture = opts.capture,
		profile = opts.profile or self.state.config.context.handoff.profile,
		max_chars = self.state.config.context.handoff.max_chars,
		diff = capture.diff(workspace.root),
		note = opts.note,
		summary = opts.summary,
		transcript = opts.transcript,
	})
end

function Workflow:open_conversation(run)
	conversation.open({
		provider = run.provider,
		session_id = run.session and run.session.id or run.id,
		run_id = run.id,
		state = run.state,
		on_input = function(message)
			self:send(run.id, message)
		end,
		on_cancel = function()
			self:cancel(run.id)
		end,
		on_detach = function()
			self:update(run.id, { state = "detached" })
		end,
		on_message = function(role, message)
			if run.transcript == "available" then
				self.transcripts[run.id] = self.transcripts[run.id] or {}
				table.insert(self.transcripts[run.id], "## " .. role .. "\n" .. message)
			end
		end,
	})
end

function Workflow:open_terminal(run, prepared)
	local opened = self.terminal:open({
		id = run.id,
		cwd = run.workspace.root,
		command = prepared.command,
		on_exit = function(result)
			local latest = self.store:get(run.id)
			if latest and active_state(latest) then
				self:update(run.id, { state = result.code == 0 and "completed" or "failed" })
			end
			self.active[run.id] = nil
		end,
	})
	self.active[run.id] = { kind = "terminal", terminal_id = run.id }
	return self:update(run.id, {
		state = "running",
		session = { id = prepared.session.id, resume_supported = true },
		process = { job_id = opened.job_id },
	})
end

function Workflow:open_structured(run, prompt)
	local opened = self.structured:open({
		provider = run.provider,
		cwd = run.workspace.root,
		prompt = prompt,
		on_session = function(session)
			self:update(run.id, { session = session, state = "running" })
			pcall(conversation.update, { run_id = run.id, session_id = session.id, state = "running" })
		end,
		on_event = function(kind, value)
			if kind == "text" then
				pcall(conversation.update, { run_id = run.id, text = value, state = "running" })
			elseif kind == "settled" then
				self:update(run.id, { state = "waiting_input" })
				pcall(conversation.update, { run_id = run.id, state = "waiting_input" })
			elseif kind == "error" then
				pcall(conversation.update, { run_id = run.id, text = value, state = "failed" })
			end
		end,
		on_usage = function(usage)
			self:update(run.id, { usage = usage })
		end,
		on_exit = function(result)
			local latest = self.store:get(run.id)
			if latest and active_state(latest) then
				self:update(run.id, { state = result.code == 0 and "completed" or "failed" })
			end
			self.active[run.id] = nil
		end,
	})
	self.active[run.id] = { kind = "structured", handle = opened }
	self:open_conversation(run)
	return run
end

function Workflow:open_managed(run, prompt)
	local history
	if self.providers[run.provider].managed_mode == "history" then
		local directory = self.store.directory .. "/aider"
		vim.fn.mkdir(directory, "p")
		history = directory .. "/" .. run.id .. ".md"
	end
	local reference
	local function permission(request, respond)
		vim.ui.select({ "approved", "denied", "cancelled" }, {
			prompt = "Gator approval · " .. request.provider .. " · " .. request.action,
		}, function(decision)
			respond(decision or "cancelled")
		end)
	end
	local opened = self.managed:open({
		provider = run.provider,
		cwd = run.workspace.root,
		task_id = run.id,
		history = history,
		prompt = prompt,
		on_session = function(session)
			reference = session
			self.active[run.id] = { kind = "managed", reference = session }
			self:update(run.id, {
				state = "running",
				session = { id = session.id, resume_supported = managed_adapter.can_resume(run.provider) },
			})
			pcall(conversation.update, { run_id = run.id, session_id = session.id, state = "running" })
		end,
		on_event = function(event)
			if event.type == "text" or event.type == "complete" then
				pcall(conversation.update, { run_id = run.id, text = event.text, state = event.type == "complete" and "waiting_input" or "running" })
				if event.type == "complete" then
					self:update(run.id, { state = "waiting_input" })
				end
			elseif event.type == "error" then
				pcall(conversation.update, { run_id = run.id, text = event.text, state = "failed" })
			end
		end,
		on_permission = permission,
		on_exit = function(result)
			local latest = self.store:get(run.id)
			if latest and active_state(latest) then
				self:update(run.id, { state = result.stopped and "stopped" or (result.code == 0 and "completed" or "failed") })
			end
			self.active[run.id] = nil
		end,
	})
	if reference then
		self.active[run.id] = { kind = "managed", reference = reference }
	end
	self:open_conversation(run)
	return opened
end

function Workflow:launch(opts)
	if type(opts) ~= "table" then
		fail("launch requires options")
	end
	local objective = text(opts.objective, "objective")
	local chosen = self:provider(opts.provider)
	local transport = self:resolve_transport(chosen, opts.transport)
	local id = run_store.id("run")
	local workspace = self:workspace(id, opts.force_worktree == true)
	local body, usage = self:bundle(opts, workspace)
	local bundle_id = run_store.id("bundle")
	self.store:bundle(bundle_id, body)
	if workspace.kind == "worktree" then
		self.store:materialize_bundle(bundle_id, body, workspace.root)
	end
	local run = self:put({
		id = id,
		provider = chosen.provider,
		role = opts.role or "primary",
		transport = transport,
		state = "starting",
		workspace = workspace,
		parent_run_id = opts.parent_run_id,
		bundle_id = bundle_id,
		objective = objective,
		transcript = transport == "terminal" and "unavailable" or "available",
		usage = usage,
		created_at = now(),
		updated_at = now(),
	})
	local prompt = objective .. "\n\nUse this Gator context bundle:\n\n" .. body
	if transport == "terminal" then
		self.bridge:start({ provider = run.provider, cwd = workspace.root, prompt = prompt }, function(prepared, reason)
			if not prepared then
				self:update(run.id, { state = "failed" })
				vim.notify("Gator launch: " .. tostring(reason), vim.log.levels.ERROR)
				return
			end
			local ok, err = pcall(self.open_terminal, self, run, prepared)
			if not ok then
				self:update(run.id, { state = "failed" })
				vim.notify(tostring(err), vim.log.levels.ERROR, { title = "Gator" })
			end
		end)
	elseif structured.supports(run.provider) then
		self:open_structured(run, prompt)
	else
		self:open_managed(run, prompt)
	end
	if opts.remember ~= false then
		self.store:set_default_provider(run.provider)
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
	self:update(run.id, { state = "running" })
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
		return self:open_conversation(run)
	end
	return self:resume(id)
end

function Workflow:resume(id)
	local run = self:run(id)
	if run.transport ~= "terminal" or not run.session or not run.session.resume_supported then
		fail("this run cannot be resumed through Gator")
	end
	local ok = pcall(self.terminal.attach, self.terminal, id)
	if ok then
		return true
	end
	self.bridge:resume({ provider = run.provider, session = { provider = run.provider, id = run.session.id, owner = "provider" } }, function(prepared, reason)
		if not prepared then
			vim.notify("Gator resume: " .. tostring(reason), vim.log.levels.ERROR)
			return
		end
		self:open_terminal(run, prepared)
	end)
	return true
end

function Workflow:transcript(run)
	local value = self.transcripts[run.id]
	return value and table.concat(value, "\n\n") or nil
end

function Workflow:handoff(source_id, target, opts)
	opts = opts or {}
	local source = self:run(source_id)
	if not target then
		local choices = {}
		for _, value in pairs(self.providers) do
			if value.provider ~= source.provider then
				table.insert(choices, value)
			end
		end
		return provider_picker.open({
			providers = choices,
			on_launch = function(choice)
				self:handoff(source_id, choice.provider, opts)
			end,
		})
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
	body = vim.fn.filereadable(bundle_path) == 1 and table.concat(vim.fn.readfile(bundle_path), "\n") or source.objective
	if type(opts.summary) == "string" and vim.trim(opts.summary) ~= "" then
		body = body .. "\n\n## Source-agent summary\n" .. opts.summary
	end
	if profile == "compact" then
		body = body:sub(1, self.state.config.context.handoff.max_chars)
	elseif profile == "full" then
		local transcript = self:transcript(source)
		if transcript then
			body = body .. "\n\n## Gator-owned transcript\n" .. transcript
		end
	elseif profile == "summary-first" then
		vim.ui.input({ prompt = "Source-agent handoff summary: " }, function(summary)
			if type(summary) == "string" and vim.trim(summary) ~= "" then
				self:handoff(source_id, target, { profile = "compact", summary = summary })
			end
		end)
		return true
	end
	handoff_review.open({
		source = source,
		target = target,
		profile = profile,
		body = body,
		on_confirm = function(reviewed)
			local ok, err = pcall(self.launch, self, {
				objective = "Continue the reviewed handoff from " .. source.provider,
				provider = target,
				bundle_body = reviewed,
				parent_run_id = source.id,
				role = "writer",
				force_worktree = writers(source),
				remember = false,
			})
			if not ok then
				vim.notify(tostring(err), vim.log.levels.ERROR, { title = "Gator" })
			end
		end,
	})
	return true
end

function Workflow:launch_parallel(id)
	local source = self:run(id)
	return self:handoff(source.id, nil, { profile = "compact" })
end

function Workflow:open_runs()
	return run_graph.open(self)
end

function Workflow:close()
	for id in pairs(vim.deepcopy(self.active)) do
		pcall(self.stop, self, id)
	end
	self.managed:shutdown()
	return true
end

return M
