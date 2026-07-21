local provider_event = require("gator.core.provider_event")
local redact = require("gator.policy.redact")
local runtime = require("gator.core.runtime")

local M = { api_version = 1 }
local Bus = {}
local Subscription = {}

Bus.__index = Bus
Subscription.__index = Subscription

local function fail(message)
	error("Gator runtime events: " .. redact.text(tostring(message)), 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function event_type(value, name)
	if type(value) ~= "string" then
		fail(name .. " must be a provider event type")
	end
	local domain, action = value:match("^([a-z][a-z0-9_-]*)%.([a-z][a-z0-9_-]*)$")
	if not domain or not action or not provider_event.domains[domain] then
		fail(name .. " has an unsupported domain")
	end
	return value
end

local function types(value)
	if value == nil then
		return nil
	end
	if type(value) ~= "table" or not vim.islist(value) or #value == 0 then
		fail("types must be a non-empty array")
	end
	local result = {}
	for index, value in ipairs(value) do
		value = event_type(value, "types " .. index)
		if result[value] then
			fail("types must not contain duplicates")
		end
		result[value] = true
	end
	return result
end

local function status(self)
	local listeners = 0
	for _ in pairs(self.listeners) do
		listeners = listeners + 1
	end
	return vim.deepcopy({
		state = self.state,
		pending = #self.pending,
		listeners = listeners,
		delivered = self.delivered,
		failures = self.failures,
		last_failure = self.last_failure,
	})
end

function M.new(opts)
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	for key in pairs(opts) do
		if key ~= "runtime" and key ~= "id" and key ~= "schedule" and key ~= "limit" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	if not runtime.is(opts.runtime) then
		fail("new requires a runtime owner")
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
		id = identifier(opts.id or "runtime-events", "service id"),
		schedule = opts.schedule or vim.schedule,
		limit = limit,
		state = "unavailable",
		listeners = {},
		pending = {},
		next_id = 0,
		delivered = 0,
		failures = 0,
		scheduled = false,
	}, Bus)
	value.runtime:register(value.id, {
		start = function()
			value.state = "ready"
			value:request_drain()
			return value
		end,
		stop = function()
			value.state = "cancelled"
			value.pending = {}
			value.scheduled = false
			value.listeners = {}
		end,
	})
	return value
end

function M.is(value)
	return getmetatable(value) == Bus
end

function M.is_subscription(value)
	return getmetatable(value) == Subscription
end

function Bus:start()
	if not M.is(self) then
		fail("start requires a runtime event bus")
	end
	self.runtime:start(self.id)
	return self:status()
end

function Bus:stop(reason)
	if not M.is(self) then
		fail("stop requires a runtime event bus")
	end
	self.runtime:stop(self.id, reason or "cancelled")
	return self:status()
end

function Bus:status()
	if not M.is(self) then
		fail("status requires a runtime event bus")
	end
	return status(self)
end

function Bus:subscribe(opts)
	if not M.is(self) then
		fail("subscribe requires a runtime event bus")
	end
	if type(opts) ~= "table" or type(opts.handler) ~= "function" then
		fail("subscribe requires a handler")
	end
	for key in pairs(opts) do
		if key ~= "types" and key ~= "handler" then
			fail("subscribe contains unsupported field: " .. tostring(key))
		end
	end
	self.next_id = self.next_id + 1
	local id = "runtime-listener-" .. self.next_id
	self.listeners[id] = { types = types(opts.types), handler = opts.handler }
	return setmetatable({ id = id, bus = self, cancelled = false }, Subscription)
end

function Subscription:status()
	if not M.is_subscription(self) then
		fail("subscription status requires a runtime event subscription")
	end
	return { state = self.cancelled and "cancelled" or self.bus.state, id = self.id }
end

function Subscription:cancel()
	if not M.is_subscription(self) then
		fail("subscription cancel requires a runtime event subscription")
	end
	if self.cancelled then
		return false
	end
	self.bus.listeners[self.id] = nil
	self.cancelled = true
	return true
end

function Bus:request_drain()
	if not M.is(self) then
		fail("request_drain requires a runtime event bus")
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
		self.last_failure = redact.text(tostring(detail))
		return false
	end
	return true
end

function Bus:publish(value)
	if not M.is(self) then
		fail("publish requires a runtime event bus")
	end
	if self.state ~= "ready" then
		return self:status()
	end
	if not provider_event.is(value) then
		fail("publish requires a normalized provider event")
	end
	if #self.pending >= self.limit then
		return { state = "failed", reason = "runtime event queue is full" }
	end
	table.insert(self.pending, provider_event.from_record(provider_event.to_record(value)))
	self:request_drain()
	return self:status()
end

function Bus:append(value)
	return self:publish(value)
end

function Bus:drain()
	if not M.is(self) then
		fail("drain requires a runtime event bus")
	end
	self.scheduled = false
	while self.state == "ready" and #self.pending > 0 do
		local value = table.remove(self.pending, 1)
		for _, listener in pairs(self.listeners) do
			if not listener.types or listener.types[value.type] then
				local ok, detail = pcall(listener.handler, provider_event.from_record(provider_event.to_record(value)))
				if ok then
					self.delivered = self.delivered + 1
				else
					self.failures = self.failures + 1
					self.last_failure = redact.text(tostring(detail))
				end
			end
		end
	end
	return self:status()
end

return M
