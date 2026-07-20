local redact = require("gator.policy.redact")

local M = {}
local Cancellation = {}

Cancellation.__index = Cancellation

local function fail(message)
	error("Gator coordinator cancellation: " .. message, 3)
end

local function reason(value)
	if value == nil then
		return "cancelled"
	end
	if type(value) ~= "string" or value == "" then
		fail("reason must be a non-empty string")
	end
	return redact.text(value)
end

function M.new()
	return setmetatable({ callbacks = {}, cancelled = false }, Cancellation)
end

function M.is(value)
	return getmetatable(value) == Cancellation
end

function Cancellation:status()
	if not M.is(self) then
		fail("status requires a cancellation token")
	end
	return { cancelled = self.cancelled, reason = self.reason }
end

function Cancellation:on_cancel(callback)
	if not M.is(self) then
		fail("on_cancel requires a cancellation token")
	end
	if type(callback) ~= "function" then
		fail("on_cancel requires a callback")
	end
	if self.cancelled then
		callback(self:status())
		return false
	end
	table.insert(self.callbacks, callback)
	return true
end

function Cancellation:cancel(value)
	if not M.is(self) then
		fail("cancel requires a cancellation token")
	end
	if self.cancelled then
		return false
	end
	self.cancelled = true
	self.reason = reason(value)
	for index, callback in ipairs(self.callbacks) do
		local ok, err = xpcall(function()
			callback(self:status())
		end, debug.traceback)
		if not ok then
			fail("cancellation callback " .. index .. " failed: " .. redact.text(err))
		end
	end
	return true
end

return M
