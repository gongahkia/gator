local capabilities = require("gator.adapters.capabilities")
local managed_adapter = require("gator.adapters.managed")
local native_terminal = require("gator.adapters.native_terminal")
local terminal = require("gator.adapters.terminal")
local task = require("gator.core.task")
local task_file = require("gator.core.task_file")
local lifecycle = require("gator.core.lifecycle")
local session = require("gator.core.session")
local health = require("gator.health")
local palette = require("gator.ui.palette")
local provider_picker = require("gator.ui.provider_picker")
local conversation = require("gator.ui.conversation")
local approval_details = require("gator.ui.approval_details")
local approval_event = require("gator.core.approval_event")
local state_store = require("gator.state")

local M = { name = "workflow", api_version = 1 }
local Workflow = {}

Workflow.__index = Workflow

local function fail(message)
	error("Gator workflow: " .. tostring(message), 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function project_root(value)
	if value ~= nil then
		if type(value) ~= "string" or value == "" then
			fail("root must be a non-empty path")
		end
		local resolved = vim.uv.fs_realpath(value)
		if not resolved or vim.fn.isdirectory(resolved) ~= 1 then
			fail("root must resolve to a directory")
		end
		return resolved
	end
	local result = vim.system({ "git", "rev-parse", "--show-toplevel" }, { cwd = vim.fn.getcwd(), text = true }):wait()
	local root = result.code == 0 and vim.trim(result.stdout or "") or ""
	return project_root(root)
end

local function slug(value)
	value = value:lower():gsub("[^a-z0-9]+", "-"):gsub("^-+", ""):gsub("-+$", "")
	if value == "" then
		value = "task"
	end
	if not value:match("^[a-z]") then
		value = "task-" .. value
	end
	return value:sub(1, 63):gsub("-+$", "")
end

local function records(state)
	local result = {}
	for _, value in ipairs(state.tasks) do
		result[value.id] = value
	end
	return result
end

local function catalog_contract(record, transport_name)
	local ready = { available = true, modes = { transport_name } }
	local unavailable = { available = false, reason = "unavailable for this Gator workflow" }
	local auth = record.readiness_state == "user_confirmed" and { available = true, modes = { "user_confirmed" } }
		or ready
	local permission = transport_name == "managed"
			and record.mode == "acp"
			and { available = true, modes = { "user_decision" } }
		or unavailable
	return capabilities.new({
		provider = record.provider,
		transport = ready,
		auth = auth,
		session = ready,
		permission = permission,
		model = unavailable,
		command = unavailable,
		tool = unavailable,
		context = unavailable,
		usage = unavailable,
	})
end

function M.new(opts)
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
		then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	if opts.terminal ~= nil and type(opts.terminal.open) ~= "function" then
		fail("terminal must open native terminal sessions")
	end
	if opts.bridge ~= nil and (type(opts.bridge.start) ~= "function" or type(opts.bridge.resume) ~= "function") then
		fail("bridge must create and resume native sessions")
	end
	if opts.readiness ~= nil and type(opts.readiness) ~= "function" then
		fail("readiness must be a function")
	end
	if
		opts.managed ~= nil
		and (
			type(opts.managed.open) ~= "function"
			or type(opts.managed.send) ~= "function"
			or type(opts.managed.cancel) ~= "function"
			or type(opts.managed.stop) ~= "function"
			or type(opts.managed.shutdown) ~= "function"
			or type(opts.managed.is_active) ~= "function"
		)
	then
		fail("managed must implement open, send, cancel, and is_active")
	end
	local value = setmetatable({
		state = opts.state,
		root = project_root(opts.root),
		terminal = opts.terminal or terminal.new(),
		bridge = opts.bridge or native_terminal.new(),
		readiness = opts.readiness or health.launch_catalog,
		managed = opts.managed or managed_adapter.new({ shutdown = true }),
		active = {},
		palette = {},
		providers = {},
		provider_modes = {},
		permission_sequence = 0,
	}, Workflow)
	value:load()
	value:refresh()
	return value
end

function Workflow:task_path(id)
	return self.root .. "/.gator/tasks/" .. identifier(id, "task id") .. ".md"
end

function Workflow:active_task_id()
	return self.active[vim.api.nvim_get_current_tabpage()] or self.state.active_task_id
end

function Workflow:select(id)
	id = identifier(id, "task id")
	if not records(self.state)[id] then
		fail("task is unavailable: " .. id)
	end
	self.active[vim.api.nvim_get_current_tabpage()] = id
	self.state:update({ active_task_id = id })
	return id
end

function Workflow:task(id)
	id = identifier(id or self:active_task_id(), "task id")
	local value = records(self.state)[id]
	if not value then
		fail("task is unavailable: " .. id)
	end
	return task.from_record(task.to_record(value))
end

function Workflow:write(value)
	return task_file.write(self:task_path(value.id), value)
end

function Workflow:replace(value)
	self.state:mutate(function(next)
		for index, existing in ipairs(next.tasks) do
			if existing.id == value.id then
				next.tasks[index] = task.from_record(task.to_record(value))
				return
			end
		end
		table.insert(next.tasks, task.from_record(task.to_record(value)))
		table.sort(next.tasks, function(left, right)
			return left.id < right.id
		end)
	end)
	self:register_palette()
	return value
end

function Workflow:create(objective)
	if type(objective) ~= "string" or vim.trim(objective) == "" then
		fail("task objective must be non-empty text")
	end
	local known, base = records(self.state), slug(objective)
	local id, suffix = base, 1
	id = base
	while known[id] do
		suffix = suffix + 1
		id = base .. "-" .. suffix
	end
	local value = task.new({
		id = id,
		objective = objective,
		workspace = { kind = "project", root = self.root },
		created_at = os.time(),
		updated_at = os.time(),
	})
	self:write(value)
	self:replace(value)
	self:select(id)
	return value
end

function Workflow:prompt_create()
	vim.ui.input({ prompt = "Gator task: " }, function(value)
		if type(value) == "string" and vim.trim(value) ~= "" then
			local ok, created = pcall(self.create, self, value)
			if not ok then
				vim.notify(tostring(created), vim.log.levels.ERROR, { title = "Gator" })
			end
		end
	end)
end

function Workflow:load()
	local found, failures, seen = {}, {}, {}
	for _, path in ipairs(vim.fn.glob(self.root .. "/.gator/tasks/*.md", false, true)) do
		local ok, value = pcall(function()
			return task_file.parse(table.concat(vim.fn.readfile(path), "\n"))
		end)
		if not ok then
			table.insert(failures, path .. ": " .. tostring(value))
		elseif seen[value.id] then
			table.insert(failures, path .. ": duplicate task id " .. value.id)
		else
			seen[value.id] = true
			table.insert(found, value)
		end
	end
	table.sort(found, function(left, right)
		return left.id < right.id
	end)
	self.state:update({ tasks = found })
	self:register_palette()
	return { tasks = #found, failures = failures }
end

function Workflow:refresh()
	local available, modes = {}, {}
	for _, value in
		ipairs(self.readiness({
			cwd = self.root,
			pi_user_confirmed = self.state.config.providers.pi.user_confirmed,
		}))
	do
		if value.available and native_terminal.supports(value.provider) then
			available[value.provider] = catalog_contract(value, "native")
			modes[value.provider] = "terminal"
		end
	end
	local confirmations = {}
	for name, provider in pairs(self.state.config.providers) do
		confirmations[name] = provider.user_confirmed
	end
	for _, value in ipairs(managed_adapter.catalog({ cwd = self.root, user_confirmed = confirmations })) do
		if value.available then
			available[value.provider] = catalog_contract(value, "managed")
			modes[value.provider] = value.mode
		end
	end
	self.providers = available
	self.provider_modes = modes
	self:register_palette()
	return vim.deepcopy(available)
end

function Workflow:transition(value, target)
	if value.lifecycle == target then
		return value
	end
	if value.lifecycle == "draft" and target == "running" then
		value = lifecycle.transition(value, "planned")
	end
	if value.lifecycle == "failed" and target == "running" then
		value = lifecycle.transition(value, "planned")
	end
	return lifecycle.transition(value, target)
end

function Workflow:history_path(value)
	local directory = self.root .. "/.gator/aider"
	if vim.fn.mkdir(directory, "p") ~= 1 and vim.fn.isdirectory(directory) ~= 1 then
		fail("cannot create managed Aider history directory")
	end
	return directory .. "/" .. value.id .. ".md"
end

function Workflow:managed_reference(task_id, provider_name)
	local value = self:task(task_id)
	for _, reference in ipairs(value.sessions) do
		if reference.provider == provider_name and reference.mode == self.provider_modes[provider_name] then
			return reference
		end
	end
	return nil
end

function Workflow:mark_running(value)
	local next_value = self:transition(value, "running")
	self:write(next_value)
	self:replace(next_value)
	self:select(next_value.id)
	return next_value
end

function Workflow:managed_callbacks(value, provider_name)
	local run_id = value.id .. "-managed"
	return {
		on_session = function(reference)
			local current = records(self.state)[value.id]
			if not current then
				return
			end
			local linked = session.link(
				current,
				session.new({
					task_id = value.id,
					provider = reference.provider,
					id = reference.id,
					owner = reference.owner,
					mode = reference.mode,
				})
			)
			self:mark_running(linked)
			pcall(conversation.update, { session_id = reference.id, state = "running" })
		end,
		on_event = function(event)
			local state = event.type == "error" and "failed" or event.type == "complete" and "ready" or "running"
			pcall(conversation.update, { state = state, text = event.text })
		end,
		on_permission = function(request, respond)
			self.permission_sequence = self.permission_sequence + 1
			local provider = { name = provider_name }
			if request.session_id then
				provider.session_id = request.session_id
			end
			local ok, event = pcall(approval_event.request, {
				id = value.id .. "-approval-" .. self.permission_sequence,
				run_id = run_id,
				provider = provider,
				sequence = self.permission_sequence,
				at = os.time(),
				request_id = request.request_id,
				action = request.action,
				details = request.details,
			})
			if not ok then
				respond("cancelled")
				return
			end
			local opened = pcall(approval_details.open, {
				request = event,
				on_decide = function(decision)
					return respond(decision.decision)
				end,
			})
			if not opened then
				respond("cancelled")
			end
		end,
		on_resume_fallback = function(request)
			if type(request.session) ~= "table" then
				vim.notify("Gator attach: " .. tostring(request.reason), vim.log.levels.ERROR)
				return
			end
			pcall(conversation.close)
			local ok, failure = pcall(self.open_terminal, self, value, {
				session = request.session,
				command = { "copilot", "--resume", request.session.id },
			}, true)
			if not ok then
				vim.notify("Gator Copilot terminal fallback: " .. tostring(failure), vim.log.levels.ERROR)
				return
			end
			vim.notify("Gator Copilot: ACP reattach unavailable; opened CLI resume fallback", vim.log.levels.WARN)
		end,
		on_exit = function(result)
			if result.fallback or result.stopped then
				return
			end
			local current = records(self.state)[value.id]
			if not current or current.lifecycle ~= "running" then
				return
			end
			local next_value = self:transition(current, result.code == 0 and "awaiting_review" or "failed")
			self:write(next_value)
			self:replace(next_value)
			pcall(conversation.update, { state = result.code == 0 and "ready" or "failed" })
		end,
	}
end

function Workflow:open_managed(value, provider_name, reference, prompt)
	local mode = self.provider_modes[provider_name]
	if not mode or not managed_adapter.supports(provider_name) then
		fail("provider is unavailable for managed launch: " .. provider_name)
	end
	local callbacks = self:managed_callbacks(value, provider_name)
	local history = mode == "history" and self:history_path(value) or nil
	local started = mode == "acp" or (type(prompt) == "string" and vim.trim(prompt) ~= "")
	if started then
		value = self:mark_running(value)
	end
	local ok, opened = pcall(self.managed.open, self.managed, {
		provider = provider_name,
		cwd = value.workspace.root,
		task_id = value.id,
		session = reference,
		history = history,
		prompt = prompt,
		on_session = callbacks.on_session,
		on_event = callbacks.on_event,
		on_permission = callbacks.on_permission,
		on_resume_fallback = callbacks.on_resume_fallback,
		on_exit = callbacks.on_exit,
	})
	if not ok then
		local current = records(self.state)[value.id]
		if current and current.lifecycle == "running" then
			local failed = self:transition(current, "failed")
			self:write(failed)
			self:replace(failed)
		end
		fail(opened)
	end
	if opened.fallback then
		return opened
	end
	local initial = reference and reference.id or (mode == "history" and history or value.id)
	conversation.open({
		provider = provider_name,
		session_id = initial,
		state = "starting",
		on_input = function(text)
			self:prompt_session({ task_id = value.id, provider = provider_name, id = initial }, text)
		end,
		on_cancel = function()
			local current = self:managed_reference(value.id, provider_name)
			return current and self.managed:cancel(current) or false
		end,
		on_detach = function()
			vim.notify("Gator session detached; use Attach session to reopen it", vim.log.levels.INFO)
		end,
	})
	return opened
end

function Workflow:prompt_session(reference, prompt)
	if type(reference) ~= "table" or type(reference.task_id) ~= "string" then
		fail("session input must identify its task")
	end
	if type(prompt) ~= "string" or vim.trim(prompt) == "" then
		fail("session input must be non-empty text")
	end
	local value = self:task(reference.task_id)
	local linked = self:managed_reference(value.id, reference.provider)
	if not linked then
		fail("linked managed session is unavailable")
	end
	if value.lifecycle ~= "running" then
		value = self:mark_running(value)
	end
	if self.managed:is_active(linked) then
		self.managed:send(linked, prompt)
		return true
	end
	self:open_managed(value, linked.provider, linked, prompt)
	return true
end

function Workflow:open_terminal(value, prepared, resumed)
	local terminal_id = value.id .. "-" .. prepared.session.provider
	local opened = self.terminal:open({
		id = terminal_id,
		cwd = value.workspace.root,
		command = prepared.command,
		on_exit = function(result)
			local current = records(self.state)[value.id]
			if not current or current.lifecycle ~= "running" then
				return
			end
			local next = self:transition(current, result.code == 0 and "awaiting_review" or "failed")
			self:write(next)
			self:replace(next)
		end,
	})
	local linked = session.link(
		value,
		session.new({
			task_id = value.id,
			provider = prepared.session.provider,
			id = prepared.session.id,
			owner = "provider",
		})
	)
	if not resumed then
		linked = self:transition(linked, "planned")
	end
	linked = self:transition(linked, "running")
	self:write(linked)
	self:replace(linked)
	self:select(linked.id)
	return opened
end

function Workflow:launch(provider_name)
	local value = self:task()
	provider_name = identifier(provider_name, "provider")
	if not self.providers[provider_name] then
		fail("provider is unavailable for launch: " .. provider_name)
	end
	if self.provider_modes[provider_name] ~= "terminal" then
		local ok, failure = pcall(self.open_managed, self, value, provider_name, nil, value.objective)
		if not ok then
			vim.notify(tostring(failure), vim.log.levels.ERROR, { title = "Gator" })
		end
		return
	end
	self.bridge:start(
		{ provider = provider_name, cwd = value.workspace.root, prompt = value.objective },
		function(prepared, reason)
			if not prepared then
				vim.notify("Gator launch: " .. tostring(reason), vim.log.levels.ERROR)
				return
			end
			local ok, failure = pcall(self.open_terminal, self, value, prepared, false)
			if not ok then
				vim.notify(tostring(failure), vim.log.levels.ERROR, { title = "Gator" })
			end
		end
	)
end

function Workflow:attach()
	local value = self:task()
	local reference = value.sessions[1]
	if not reference then
		fail("selected task has no linked provider session")
	end
	if reference.mode ~= "terminal" then
		if not managed_adapter.can_resume(reference.provider) then
			fail("provider does not document managed-session resume: " .. reference.provider)
		end
		local ok, failure = pcall(self.open_managed, self, value, reference.provider, reference, nil)
		if not ok then
			vim.notify(tostring(failure), vim.log.levels.ERROR, { title = "Gator" })
		end
		return true
	end
	local terminal_id = value.id .. "-" .. reference.provider
	local ok = pcall(self.terminal.attach, self.terminal, terminal_id)
	if ok then
		return true
	end
	self.bridge:resume({ provider = reference.provider, session = reference }, function(prepared, reason)
		if not prepared then
			vim.notify("Gator attach: " .. tostring(reason), vim.log.levels.ERROR)
			return
		end
		local opened, failure = pcall(self.open_terminal, self, value, prepared, true)
		if not opened then
			vim.notify(tostring(failure), vim.log.levels.ERROR, { title = "Gator" })
		end
	end)
	return true
end

function Workflow:stop_session()
	local value = self:task()
	for _, reference in ipairs(value.sessions) do
		if reference.mode ~= "terminal" and self.managed:is_active(reference) then
			if not self.managed:stop(reference) then
				fail("managed session could not be stopped")
			end
			pcall(conversation.close)
			vim.notify("Gator session stopped; its provider session remains resumable", vim.log.levels.INFO)
			return true
		end
	end
	fail("selected task has no active managed session")
end

function Workflow:open_provider_picker()
	local providers = {}
	for _, value in pairs(self.providers) do
		table.insert(providers, value)
	end
	provider_picker.open({
		providers = providers,
		on_launch = function(value)
			self:launch(value.provider)
		end,
	})
end

function Workflow:register_palette()
	for _, id in ipairs(self.palette) do
		palette.unregister(id)
	end
	self.palette = {}
	local function add(kind, name, execute)
		table.insert(self.palette, palette.register({ kind = kind, name = name, execute = execute }))
	end
	add("action", "create-task", function()
		self:prompt_create()
	end)
	add("action", "import-tasks", function()
		local report = self:load()
		if #report.failures > 0 then
			vim.notify(table.concat(report.failures, "\n"), vim.log.levels.WARN, { title = "Gator" })
		end
	end)
	add("action", "launch-task", function()
		self:open_provider_picker()
	end)
	add("action", "attach-session", function()
		self:attach()
	end)
	add("action", "stop-session", function()
		self:stop_session()
	end)
	add("action", "refresh-providers", function()
		self:refresh()
	end)
	for _, value in ipairs(self.state.tasks) do
		add("task", value.id, function()
			self:select(value.id)
		end)
	end
	for name in pairs(self.providers) do
		add("provider", name, function()
			self:launch(name)
		end)
	end
end

function Workflow:close()
	for _, id in ipairs(self.palette) do
		palette.unregister(id)
	end
	self.palette = {}
	self.managed:shutdown()
	return true
end

return M
