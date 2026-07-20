local compat = require("gator.compat")
local config = require("gator.config")
local state = require("gator.state")
local ui = require("gator.ui")
local motion = require("gator.ui.motion")
local accessibility = require("gator.ui.accessibility")
local redact = require("gator.policy.redact")
local consent = require("gator.telemetry.consent")

local M = { name = "coordinator", api_version = 1 }
local Coordinator = {}
Coordinator.__index = Coordinator

local function fail(message)
	error("Gator coordinator: " .. message, 3)
end

local function configure(settings)
	motion.configure(settings.ui.motion)
	accessibility.configure(settings.ui)
	redact.configure({ patterns = settings.telemetry.redaction_patterns })
	consent.configure({ enabled = settings.telemetry.enabled })
end

function M.new(opts)
	local report = compat.require_supported()
	local settings = config.resolve(opts)
	configure(settings)
	local value = setmetatable({ _state = state.new(settings) }, Coordinator)
	value._state.compatibility = report
	return value
end

function M.is(value)
	return getmetatable(value) == Coordinator
end

function Coordinator:state()
	if not M.is(self) or type(self._state) ~= "table" then
		fail("state requires an initialized coordinator")
	end
	return self._state
end

function Coordinator:open()
	return ui.open(self:state())
end

function Coordinator:health()
	self:state()
	vim.cmd("checkhealth gator")
end

return M
