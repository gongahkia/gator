local dependencies = require("gator.coordinator.dependencies")

local M = { name = "coordinator", api_version = 1 }
local Coordinator = {}
Coordinator.__index = Coordinator

local function fail(message)
	error("Gator coordinator: " .. message, 3)
end

local function configure(container, settings)
	container:require("motion").configure(settings.ui.motion)
	container:require("accessibility").configure(settings.ui)
	container:require("redact").configure({ patterns = settings.telemetry.redaction_patterns })
	container:require("consent").configure({ enabled = settings.telemetry.enabled })
end

function M.new(opts)
	local container = dependencies.new()
	local report = container:require("compat").require_supported()
	local settings = container:require("config").resolve(opts)
	configure(container, settings)
	local value =
		setmetatable({ _dependencies = container, _state = container:require("state").new(settings) }, Coordinator)
	value._state.compatibility = report
	return value
end

function M.is(value)
	return getmetatable(value) == Coordinator
end

M.modules = dependencies.modules()

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
