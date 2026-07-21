local redact = require("gator.policy.redact")

local M = { service_states = { "registered", "running", "stopped", "cancelled", "failed" } }
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

local function service(value)
	if type(value) ~= "table" or type(value.start) ~= "function" or type(value.stop) ~= "function" then
		fail("service must expose start and stop")
	end
	return value
end

local function status(entry)
	return vim.deepcopy({
		name = entry.name,
		state = entry.state,
		started_at = entry.started_at,
		stopped_at = entry.stopped_at,
		failure = entry.failure,
	})
end

local function reason(value)
	if value == nil then
		return "shutdown"
	end
	if type(value) ~= "string" or value == "" then
		fail("service stop reason must be non-empty text")
	end
	return redact.text(value)
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
		_service_registry = {},
		service_order = {},
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

function Runtime:register(name, value)
	if not M.is(self) then
		fail("register requires a runtime")
	end
	name = identifier(name, "service name")
	service(value)
	if self._service_registry[name] then
		fail("service is already registered: " .. name)
	end
	self._service_registry[name] = { name = name, service = value, state = "registered" }
	table.insert(self.service_order, name)
	return status(self._service_registry[name])
end

function Runtime:status(name)
	if not M.is(self) then
		fail("status requires a runtime")
	end
	name = identifier(name, "service name")
	if not self._service_registry[name] then
		fail("service is unavailable: " .. name)
	end
	return status(self._service_registry[name])
end

function Runtime:services()
	if not M.is(self) then
		fail("services requires a runtime")
	end
	local result = {}
	for _, name in ipairs(self.service_order) do
		table.insert(result, status(self._service_registry[name]))
	end
	return result
end

function Runtime:start(name, opts)
	if not M.is(self) then
		fail("start requires a runtime")
	end
	name = identifier(name, "service name")
	if opts == nil then
		opts = {}
	end
	if type(opts) ~= "table" then
		fail("service start options must be a table")
	end
	local entry = self._service_registry[name]
	if not entry then
		fail("service is unavailable: " .. name)
	end
	if entry.state == "running" then
		return status(entry)
	end
	local started_at = self:now()
	local ok, handle = pcall(entry.service.start, entry.service, vim.deepcopy(opts))
	if not ok then
		entry.state = "failed"
		entry.failure = redact.text(tostring(handle))
		fail("service failed to start: " .. name .. ": " .. entry.failure)
	end
	entry.handle = handle
	entry.state = "running"
	entry.started_at = started_at
	entry.stopped_at = nil
	entry.failure = nil
	return status(entry)
end

function Runtime:stop(name, value)
	if not M.is(self) then
		fail("stop requires a runtime")
	end
	name = identifier(name, "service name")
	local entry = self._service_registry[name]
	if not entry then
		fail("service is unavailable: " .. name)
	end
	if entry.state ~= "running" then
		return false
	end
	value = reason(value)
	local stopped_at = self:now()
	local ok, detail = pcall(entry.service.stop, entry.service, entry.handle, value)
	if not ok then
		entry.failure = redact.text(tostring(detail))
		fail("service failed to stop: " .. name .. ": " .. entry.failure)
	end
	entry.handle = nil
	entry.state = value == "cancelled" and "cancelled" or "stopped"
	entry.stopped_at = stopped_at
	entry.failure = nil
	return status(entry)
end

function Runtime:shutdown(value)
	if not M.is(self) then
		fail("shutdown requires a runtime")
	end
	value = reason(value)
	local result, failures = {}, {}
	for index = #self.service_order, 1, -1 do
		local name = self.service_order[index]
		if self._service_registry[name].state == "running" then
			local ok, stopped = pcall(self.stop, self, name, value)
			if ok then
				table.insert(result, stopped)
			else
				table.insert(failures, redact.text(tostring(stopped)))
			end
		end
	end
	if #failures > 0 then
		fail("service shutdown failed: " .. table.concat(failures, "; "))
	end
	return result
end

return M
