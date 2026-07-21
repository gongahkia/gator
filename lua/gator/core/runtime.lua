local redact = require("gator.policy.redact")

local M = {}
local Runtime = {}

Runtime.__index = Runtime

local function fail(message)
	error("Gator runtime: " .. redact.text(tostring(message)), 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function timestamp(value)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail("clock must return a non-negative integer timestamp")
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
		if key ~= "clock" and key ~= "identifier" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	if opts.clock ~= nil and type(opts.clock) ~= "function" then
		fail("clock must be a function")
	end
	if opts.identifier ~= nil and type(opts.identifier) ~= "function" then
		fail("identifier must be a function")
	end
	return setmetatable({
		clock = opts.clock or os.time,
		identifier = opts.identifier or function(prefix, sequence)
			return prefix .. "-" .. sequence
		end,
		sequence = 0,
	}, Runtime)
end

function M.is(value)
	return getmetatable(value) == Runtime
end

function Runtime:now()
	if not M.is(self) then
		fail("now requires a runtime")
	end
	local ok, value = pcall(self.clock)
	if not ok then
		fail("clock failed: " .. tostring(value))
	end
	return timestamp(value)
end

function Runtime:next_id(prefix)
	if not M.is(self) then
		fail("next_id requires a runtime")
	end
	prefix = identifier(prefix, "identifier prefix")
	self.sequence = self.sequence + 1
	local ok, value = pcall(self.identifier, prefix, self.sequence)
	if not ok then
		fail("identifier failed: " .. tostring(value))
	end
	return identifier(value, "identifier")
end

return M
