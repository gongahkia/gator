local redact = require("gator.policy.redact")

local M = { api_version = 1, states = { ready = true, unavailable = true, failed = true, cancelled = true } }
local Faults = {}

Faults.__index = Faults

local function fail(message)
	error("Gator transport faults: " .. redact.text(tostring(message)), 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function plan(value)
	if type(value) ~= "table" or not vim.islist(value) then
		fail("failures must be an array")
	end
	local result, seen = {}, {}
	for index, value in ipairs(value) do
		if type(value) ~= "table" then
			fail("failure " .. index .. " must be an object")
		end
		for key in pairs(value) do
			if key ~= "operation" and key ~= "occurrence" and key ~= "state" and key ~= "kind" and key ~= "reason" then
				fail("failure " .. index .. " contains unsupported field: " .. tostring(key))
			end
		end
		local operation = identifier(value.operation, "failure " .. index .. " operation")
		if type(value.occurrence) ~= "number" or value.occurrence < 1 or value.occurrence % 1 ~= 0 then
			fail("failure " .. index .. " occurrence must be a positive integer")
		end
		if value.state ~= "unavailable" and value.state ~= "failed" then
			fail("failure " .. index .. " state must be unavailable or failed")
		end
		local key = operation .. "\0" .. value.occurrence
		if seen[key] then
			fail("failures contain duplicate operation occurrences")
		end
		if type(value.kind) ~= "string" or value.kind == "" then
			fail("failure " .. index .. " kind must be non-empty text")
		end
		if type(value.reason) ~= "string" or value.reason == "" then
			fail("failure " .. index .. " reason must be non-empty text")
		end
		seen[key] = true
		result[key] = {
			operation = operation,
			occurrence = value.occurrence,
			state = value.state,
			kind = value.kind,
			reason = redact.text(value.reason),
		}
	end
	return result
end

function M.new(opts)
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	for key in pairs(opts) do
		if key ~= "failures" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	return setmetatable({ failures = plan(opts.failures or {}), calls = {}, state = "ready", triggered = 0 }, Faults)
end

function M.is(value)
	return getmetatable(value) == Faults
end

function Faults:status()
	if not M.is(self) then
		fail("status requires a transport fault controller")
	end
	return vim.deepcopy({ state = self.state, calls = self.calls, triggered = self.triggered, reason = self.reason })
end

function Faults:call(operation, callback)
	if not M.is(self) then
		fail("call requires a transport fault controller")
	end
	operation = identifier(operation, "operation")
	if type(callback) ~= "function" then
		fail("call requires a callback")
	end
	if self.state == "cancelled" then
		return { state = "cancelled", reason = self.reason }
	end
	local occurrence = (self.calls[operation] or 0) + 1
	self.calls[operation] = occurrence
	local fault = self.failures[operation .. "\0" .. occurrence]
	if fault then
		self.triggered = self.triggered + 1
		return vim.deepcopy(fault)
	end
	local ok, value = pcall(callback)
	if not ok then
		return { state = "failed", kind = "callback", reason = redact.text(tostring(value)) }
	end
	return { state = "ready", value = value }
end

function Faults:cancel(reason)
	if not M.is(self) then
		fail("cancel requires a transport fault controller")
	end
	if self.state == "cancelled" then
		return false
	end
	if reason ~= nil and (type(reason) ~= "string" or reason == "") then
		fail("cancellation reason must be non-empty text")
	end
	self.state = "cancelled"
	self.reason = redact.text(reason or "transport test cancelled")
	return true
end

return M
