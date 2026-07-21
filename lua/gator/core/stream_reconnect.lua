local redact = require("gator.policy.redact")
local runtime = require("gator.core.runtime")
local session = require("gator.core.session")

local M = { api_version = 1 }
local Reconnect = {}

Reconnect.__index = Reconnect

local function fail(message)
	error("Gator stream reconnect: " .. redact.text(tostring(message)), 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function cursor(value)
	if type(value) ~= "table" or type(value.get) ~= "function" then
		fail("cursor must expose get")
	end
	return value
end

local function status(self)
	return vim.deepcopy({
		state = self.state,
		run_id = self.run_id,
		attempts = self.attempts,
		max_attempts = self.max_attempts,
		scheduled = self.scheduled,
		last_sequence = self.last_sequence,
		reason = self.reason,
	})
end

local function failed(self, value)
	self.reason = redact.text(tostring(value))
	if self.attempts >= self.max_attempts then
		self.state = "failed"
		return false
	end
	self.state = "reconnecting"
	return self:request_reconnect()
end

function M.new(opts)
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	for key in pairs(opts) do
		if
			key ~= "runtime"
			and key ~= "id"
			and key ~= "cursor"
			and key ~= "run_id"
			and key ~= "session"
			and key ~= "reconnect"
			and key ~= "schedule"
			and key ~= "max_attempts"
		then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	if not runtime.is(opts.runtime) then
		fail("new requires a runtime owner")
	end
	if not session.is(opts.session) then
		fail("new requires a provider-owned Gator session")
	end
	if type(opts.reconnect) ~= "function" then
		fail("new requires a reconnect callback")
	end
	if opts.schedule ~= nil and type(opts.schedule) ~= "function" then
		fail("schedule must be a function")
	end
	local max_attempts = opts.max_attempts or 3
	if type(max_attempts) ~= "number" or max_attempts < 1 or max_attempts % 1 ~= 0 then
		fail("max_attempts must be a positive integer")
	end
	local value = setmetatable({
		runtime = opts.runtime,
		id = identifier(opts.id or "stream-reconnect", "service id"),
		cursor = cursor(opts.cursor),
		run_id = identifier(opts.run_id, "run_id"),
		session = opts.session,
		reconnect_callback = opts.reconnect,
		schedule = opts.schedule or vim.schedule,
		max_attempts = max_attempts,
		attempts = 0,
		state = "unavailable",
		scheduled = false,
	}, Reconnect)
	value.runtime:register(value.id, {
		start = function()
			value.state = "connecting"
			value.attempts = 0
			value.reason = nil
			value:request_reconnect()
			return value
		end,
		stop = function(_, reason)
			value.scheduled = false
			value.state = "cancelled"
			value.reason = redact.text(tostring(reason or "cancelled"))
		end,
	})
	return value
end

function M.is(value)
	return getmetatable(value) == Reconnect
end

function Reconnect:start()
	if not M.is(self) then
		fail("start requires a stream reconnect manager")
	end
	self.runtime:start(self.id)
	return self:status()
end

function Reconnect:stop(reason)
	if not M.is(self) then
		fail("stop requires a stream reconnect manager")
	end
	self.runtime:stop(self.id, reason or "cancelled")
	return self:status()
end

function Reconnect:status()
	if not M.is(self) then
		fail("status requires a stream reconnect manager")
	end
	return status(self)
end

function Reconnect:request_reconnect()
	if not M.is(self) then
		fail("request_reconnect requires a stream reconnect manager")
	end
	if
		(self.state ~= "connecting" and self.state ~= "reconnecting")
		or self.scheduled
		or self.attempts >= self.max_attempts
	then
		return false
	end
	self.scheduled = true
	local ok, detail = pcall(self.schedule, function()
		self:connect()
	end)
	if not ok then
		self.scheduled = false
		self.state = "failed"
		self.reason = redact.text(tostring(detail))
		return false
	end
	return true
end

function Reconnect:connect()
	if not M.is(self) then
		fail("connect requires a stream reconnect manager")
	end
	self.scheduled = false
	if self.state ~= "connecting" and self.state ~= "reconnecting" then
		return self:status()
	end
	local read, sequence = pcall(self.cursor.get, self.cursor, self.run_id)
	if not read or type(sequence) ~= "number" or sequence < -1 or sequence % 1 ~= 0 then
		self.state = "failed"
		self.reason = read and "cursor returned an invalid sequence" or redact.text(tostring(sequence))
		return self:status()
	end
	self.last_sequence = sequence
	self.attempts = self.attempts + 1
	local ok, connected, reason = pcall(self.reconnect_callback, session.reference(self.session), {
		run_id = self.run_id,
		after_sequence = sequence,
		next_sequence = sequence + 1,
	})
	if not ok then
		failed(self, connected)
		return self:status()
	end
	if connected == false then
		failed(self, reason or "provider reconnect rejected the cursor")
		return self:status()
	end
	self.state = "connected"
	self.reason = nil
	return self:status()
end

function Reconnect:disconnect(reason)
	if not M.is(self) then
		fail("disconnect requires a stream reconnect manager")
	end
	if self.state ~= "connected" then
		return self:status()
	end
	self.state = "reconnecting"
	self.attempts = 0
	self.reason = redact.text(tostring(reason or "stream disconnected"))
	self:request_reconnect()
	return self:status()
end

return M
