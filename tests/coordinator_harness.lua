local redact = require("gator.policy.redact")

local M = {}
local Harness = {}

Harness.__index = Harness

local function fail(message)
	error("Gator coordinator harness: " .. redact.text(tostring(message)), 3)
end

function M.new(opts)
	if opts == nil then
		opts = {}
	end
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	for key in pairs(opts) do
		if key ~= "settings" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	if opts.settings ~= nil and type(opts.settings) ~= "table" then
		fail("settings must be an object")
	end
	local gator = require("gator").setup(opts.settings)
	return setmetatable({ gator = gator, operations = {}, closed = false }, Harness)
end

function M.is(value)
	return getmetatable(value) == Harness
end

function Harness:state()
	if not M.is(self) or self.closed then
		fail("state requires an active harness")
	end
	return self.gator.inspect().state
end

function Harness:dispatch(action, opts)
	if not M.is(self) or self.closed then
		fail("dispatch requires an active harness")
	end
	return self.gator.dispatch(action, opts)
end

function Harness:operation(opts)
	if not M.is(self) or self.closed then
		fail("operation requires an active harness")
	end
	local value = self.gator._coordinator:start_operation(opts)
	self.operations[value.id] = value
	return value
end

function Harness:cleanup()
	if not M.is(self) then
		fail("cleanup requires a harness")
	end
	if self.closed then
		return false
	end
	self.gator._coordinator:cancel_all("integration harness cleanup")
	for _, operation in pairs(self.operations) do
		operation:complete()
	end
	self.gator.dispatch("close")
	self.closed = true
	self.gator.setup()
	return true
end

return M
