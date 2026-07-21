local provider_event = require("gator.core.provider_event")
local redact = require("gator.policy.redact")
local runtime = require("gator.core.runtime")

local M = { api_version = 1 }
local Fanout = {}

Fanout.__index = Fanout

local function fail(message)
	error("Gator event fanout: " .. redact.text(tostring(message)), 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function status(self)
	return {
		state = self.state,
		pending = #self.pending,
		limit = self.limit,
		delivered = self.delivered,
		failures = self.failures,
		last_failure = self.last_failure,
		reason = self.reason,
	}
end

local function record_failure(self, value)
	self.failures = self.failures + 1
	self.last_failure = redact.text(tostring(value))
end

local function target(value, name)
	if type(value) ~= "function" then
		fail(name .. " target must be a function")
	end
	return value
end

function M.new(opts)
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	for key in pairs(opts) do
		if
			key ~= "runtime"
			and key ~= "id"
			and key ~= "sidebar"
			and key ~= "panel"
			and key ~= "focused"
			and key ~= "schedule"
			and key ~= "limit"
		then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	if not runtime.is(opts.runtime) then
		fail("new requires a runtime owner")
	end
	if type(opts.focused) ~= "function" then
		fail("new requires a focused-panel callback")
	end
	if opts.schedule ~= nil and type(opts.schedule) ~= "function" then
		fail("schedule must be a function")
	end
	local limit = opts.limit or 1024
	if type(limit) ~= "number" or limit < 1 or limit % 1 ~= 0 then
		fail("limit must be a positive integer")
	end
	local value = setmetatable({
		runtime = opts.runtime,
		id = identifier(opts.id or "provider-fanout", "service id"),
		sidebar = target(opts.sidebar, "sidebar"),
		panel = target(opts.panel, "panel"),
		focused = opts.focused,
		schedule = opts.schedule or vim.schedule,
		limit = limit,
		state = "unavailable",
		pending = {},
		delivered = 0,
		failures = 0,
		scheduled = false,
	}, Fanout)
	value.runtime:register(value.id, {
		start = function()
			value.state = "ready"
			value:request_drain()
			return value
		end,
		stop = function(_, reason)
			value.pending = {}
			value.scheduled = false
			value.state = "cancelled"
			value.reason = redact.text(tostring(reason or "cancelled"))
		end,
	})
	return value
end

function M.is(value)
	return getmetatable(value) == Fanout
end

function Fanout:start()
	if not M.is(self) then
		fail("start requires an event fanout")
	end
	return self.runtime:start(self.id)
end

function Fanout:stop(reason)
	if not M.is(self) then
		fail("stop requires an event fanout")
	end
	self.runtime:stop(self.id, reason or "cancelled")
	return self:status()
end

function Fanout:status()
	if not M.is(self) then
		fail("status requires an event fanout")
	end
	return vim.deepcopy(status(self))
end

function Fanout:request_drain()
	if not M.is(self) then
		fail("request_drain requires an event fanout")
	end
	if self.state ~= "ready" or self.scheduled or #self.pending == 0 then
		return false
	end
	self.scheduled = true
	local ok, detail = pcall(self.schedule, function()
		self:drain()
	end)
	if not ok then
		self.scheduled = false
		self.pending = {}
		self.state = "failed"
		self.reason = redact.text(tostring(detail))
		return false
	end
	return true
end

function Fanout:submit(value)
	if not M.is(self) then
		fail("submit requires an event fanout")
	end
	if self.state ~= "ready" then
		return self:status()
	end
	if not provider_event.is(value) then
		fail("submit requires a normalized provider event")
	end
	if #self.pending >= self.limit then
		self.state = "failed"
		self.reason = "event fanout queue is full"
		self.pending = {}
		return self:status()
	end
	table.insert(self.pending, provider_event.from_record(provider_event.to_record(value)))
	self:request_drain()
	return self:status()
end

local function deliver(self, target, value)
	local ok, accepted, reason = pcall(target, provider_event.from_record(provider_event.to_record(value)))
	if not ok then
		record_failure(self, accepted)
		return
	end
	if accepted == false then
		record_failure(self, reason or "target rejected event")
		return
	end
	self.delivered = self.delivered + 1
end

function Fanout:drain()
	if not M.is(self) then
		fail("drain requires an event fanout")
	end
	self.scheduled = false
	while self.state == "ready" and #self.pending > 0 do
		local value = table.remove(self.pending, 1)
		deliver(self, self.sidebar, value)
		local focused, active = pcall(self.focused)
		if not focused then
			record_failure(self, active)
		elseif type(active) ~= "boolean" then
			record_failure(self, "focused-panel callback must return a boolean")
		elseif active then
			deliver(self, self.panel, value)
		end
	end
	return self:status()
end

return M
