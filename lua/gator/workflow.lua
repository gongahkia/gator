local capabilities = require("gator.adapters.capabilities")
local native_terminal = require("gator.adapters.native_terminal")
local terminal = require("gator.adapters.terminal")
local task = require("gator.core.task")
local task_file = require("gator.core.task_file")
local lifecycle = require("gator.core.lifecycle")
local session = require("gator.core.session")
local health = require("gator.health")
local palette = require("gator.ui.palette")
local provider_picker = require("gator.ui.provider_picker")
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

local function catalog_contract(record)
	local ready = { available = true, modes = { "native" } }
	local unavailable = { available = false, reason = "unavailable for native terminal workflow" }
	local auth = record.authentication == "user_confirmed" and { available = true, modes = { "user_confirmed" } }
		or ready
	return capabilities.new({
		provider = record.provider,
		transport = ready,
		auth = auth,
		session = ready,
		permission = unavailable,
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
		if key ~= "state" and key ~= "root" and key ~= "terminal" and key ~= "bridge" and key ~= "readiness" then
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
	local value = setmetatable({
		state = opts.state,
		root = project_root(opts.root),
		terminal = opts.terminal or terminal.new(),
		bridge = opts.bridge or native_terminal.new(),
		readiness = opts.readiness or health.launch_catalog,
		active = {},
		palette = {},
		providers = {},
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
	local available = {}
	for _, value in
		ipairs(self.readiness({
			cwd = self.root,
			pi_user_confirmed = self.state.config.providers.pi.user_confirmed,
		}))
	do
		if value.available and native_terminal.supports(value.provider) then
			available[value.provider] = catalog_contract(value)
		end
	end
	self.providers = available
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
		fail("provider is unavailable for native launch: " .. provider_name)
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
	return true
end

return M
