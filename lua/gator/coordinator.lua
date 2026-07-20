local dependencies = require("gator.coordinator.dependencies")

local M = { name = "coordinator", api_version = 1 }
local Coordinator = {}
Coordinator.__index = Coordinator
local actions = {
	open = { fields = {} },
	health = { fields = {} },
	close = { fields = {} },
	capture_selection = { fields = { target = true, buffer = true, first_line = true, last_line = true } },
	palette = { fields = { id = true } },
}
local action_names = { "open", "health", "close", "capture_selection", "palette" }

local function fail(message)
	error("Gator coordinator: " .. message, 3)
end

local function configure(container, settings)
	container:require("motion").configure(settings.ui.motion)
	container:require("accessibility").configure(settings.ui)
	container:require("redact").configure({ patterns = settings.telemetry.redaction_patterns })
	container:require("consent").configure({ enabled = settings.telemetry.enabled })
end

local function options(action, value)
	if value == nil then
		value = {}
	end
	if type(value) ~= "table" then
		fail(action .. " options must be a table")
	end
	for key in pairs(value) do
		if not actions[action].fields[key] then
			fail(action .. " options contain unsupported field: " .. tostring(key))
		end
	end
	return value
end

local function require_string(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty string")
	end
	return value
end

function M.new(opts)
	local container = dependencies.new()
	local report = container:require("compat").require_supported()
	local settings = container:require("config").resolve(opts)
	configure(container, settings)
	local value = setmetatable({
		_dependencies = container,
		_state = container:require("state").new(settings, report),
	}, Coordinator)
	return value
end

function M.is(value)
	return getmetatable(value) == Coordinator
end

M.modules = dependencies.modules()

function M.is_action(value)
	return type(value) == "string" and actions[value] ~= nil
end

function M.actions()
	return vim.deepcopy(action_names)
end

function M.module(name)
	return dependencies.new():module(name)
end

function Coordinator:state()
	if not M.is(self) or type(self._state) ~= "table" then
		fail("state requires an initialized coordinator")
	end
	return self._state
end

function Coordinator:open()
	return self:dependency("ui").open(self:state())
end

function Coordinator:health()
	self:state()
	vim.cmd("checkhealth gator")
end

function Coordinator:dispatch(action, opts)
	if not M.is(self) then
		fail("dispatch requires an initialized coordinator")
	end
	if not M.is_action(action) then
		fail("action is unavailable: " .. tostring(action))
	end
	opts = options(action, opts)
	if action == "open" then
		return self:open()
	end
	if action == "health" then
		return self:health()
	end
	if action == "close" then
		return self:dependency("ui").close()
	end
	if action == "capture_selection" then
		return self:dependency("ui").selection.capture(self:state(), require_string(opts.target, "target"), {
			buffer = opts.buffer,
			first_line = opts.first_line,
			last_line = opts.last_line,
		})
	end
	return self:dependency("ui").palette.execute(require_string(opts.id, "palette id"))
end

function Coordinator:dependency(name)
	if not M.is(self) or not dependencies.is(self._dependencies) then
		fail("dependency requires an initialized coordinator")
	end
	return self._dependencies:require(name)
end

function Coordinator:module(name)
	if not M.is(self) or not dependencies.is(self._dependencies) then
		fail("module requires an initialized coordinator")
	end
	return self._dependencies:module(name)
end

return M
