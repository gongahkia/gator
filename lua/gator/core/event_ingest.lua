local provider_event = require("gator.core.provider_event")
local redact = require("gator.policy.redact")
local runtime = require("gator.core.runtime")

local M = {}
local Ingest = {}

Ingest.__index = Ingest

local function fail(message)
	error("Gator event ingest: " .. redact.text(tostring(message)), 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
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
			and key ~= "sink"
			and key ~= "schedule"
			and key ~= "on_error"
			and key ~= "limit"
			and key ~= "overflow"
		then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	if not runtime.is(opts.runtime) then
		fail("new requires a runtime owner")
	end
	if type(opts.sink) ~= "table" or type(opts.sink.append) ~= "function" then
		fail("new requires an event sink append function")
	end
	if opts.schedule ~= nil and type(opts.schedule) ~= "function" then
		fail("schedule must be a function")
	end
	if opts.on_error ~= nil and type(opts.on_error) ~= "function" then
		fail("on_error must be a function")
	end
	local limit = opts.limit or 1024
	if type(limit) ~= "number" or limit < 1 or limit % 1 ~= 0 then
		fail("limit must be a positive integer")
	end
	local overflow = opts.overflow or "reject"
	if overflow ~= "reject" and overflow ~= "drop_oldest" and overflow ~= "drop_newest" then
		fail("overflow policy is unavailable: " .. tostring(overflow))
	end
	local value = setmetatable({
		runtime = opts.runtime,
		id = identifier(opts.id or "provider-events", "service id"),
		sink = opts.sink,
		schedule = opts.schedule or vim.schedule,
		on_error = opts.on_error,
		pending = {},
		limit = limit,
		overflow = overflow,
		dropped = 0,
		active = false,
		scheduled = false,
	}, Ingest)
	value.runtime:register(value.id, {
		start = function()
			value.active = true
			value:request_drain()
			return value
		end,
		stop = function()
			value.active = false
			value.pending = {}
		end,
	})
	return value
end

function M.is(value)
	return getmetatable(value) == Ingest
end

function Ingest:start()
	if not M.is(self) then
		fail("start requires an event ingester")
	end
	return self.runtime:start(self.id)
end

function Ingest:stop(reason)
	if not M.is(self) then
		fail("stop requires an event ingester")
	end
	return self.runtime:stop(self.id, reason or "cancelled")
end

function Ingest:status()
	if not M.is(self) then
		fail("status requires an event ingester")
	end
	return {
		active = self.active,
		pending = #self.pending,
		limit = self.limit,
		overflow = self.overflow,
		dropped = self.dropped,
		last_error = self.last_error,
	}
end

function Ingest:request_drain()
	if not M.is(self) then
		fail("request_drain requires an event ingester")
	end
	if not self.active or self.scheduled or #self.pending == 0 then
		return false
	end
	self.scheduled = true
	local ok, detail = pcall(self.schedule, function()
		self:drain()
	end)
	if not ok then
		self.scheduled = false
		self.last_error = redact.text(tostring(detail))
		fail("cannot schedule event ingestion: " .. self.last_error)
	end
	return true
end

function Ingest:submit(value)
	if not M.is(self) then
		fail("submit requires an event ingester")
	end
	if not self.active then
		fail("event ingester is unavailable until its runtime service starts")
	end
	if not provider_event.is(value) then
		fail("submit requires a normalized provider event")
	end
	if #self.pending >= self.limit then
		if self.overflow == "reject" then
			fail("event ingestion queue is full")
		end
		self.dropped = self.dropped + 1
		if self.overflow == "drop_newest" then
			return self:status()
		end
		table.remove(self.pending, 1)
	end
	table.insert(self.pending, provider_event.from_record(provider_event.to_record(value)))
	self:request_drain()
	return self:status()
end

function Ingest:drain()
	if not M.is(self) then
		fail("drain requires an event ingester")
	end
	self.scheduled = false
	while self.active and #self.pending > 0 do
		local value = self.pending[1]
		local ok, detail =
			pcall(self.sink.append, self.sink, provider_event.from_record(provider_event.to_record(value)))
		if not ok then
			self.last_error = redact.text(tostring(detail))
			if self.on_error then
				pcall(self.on_error, self.last_error)
			end
			return false
		end
		table.remove(self.pending, 1)
		self.last_error = nil
	end
	return true
end

return M
