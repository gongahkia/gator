local M = { api_version = 1 }
local Consent = {}

Consent.__index = Consent

local actions = { collection = true, transmission = true }

local function fail(message)
	error("Gator telemetry consent: " .. message, 3)
end

local function enabled(value)
	if type(value) ~= "boolean" then
		fail("enabled must be boolean")
	end
	return value
end

local function action(value)
	if type(value) ~= "string" or not actions[value] then
		fail("action must be collection or transmission")
	end
	return value
end

function M.new(opts)
	if opts == nil then
		opts = {}
	end
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	for key in pairs(opts) do
		if key ~= "enabled" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	local value = opts.enabled
	if value == nil then
		value = false
	else
		value = enabled(value)
	end
	return setmetatable({ enabled = value }, Consent)
end

function Consent:status()
	return { enabled = self.enabled }
end

function Consent:require(value)
	value = action(value)
	if not self.enabled then
		fail("telemetry " .. value .. " requires explicit consent")
	end
	return true
end

local default = M.new()

function M.configure(opts)
	default = M.new(opts)
	return default:status()
end

function M.status()
	return default:status()
end

function M.require(value)
	return default:require(value)
end

return M
