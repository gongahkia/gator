local consent = require("gator.telemetry.consent")
local event = require("gator.telemetry.event")
local M = { api_version = 1, schema_version = 1 }
local Aggregator = {}

Aggregator.__index = Aggregator

local function fail(message)
	error("Gator telemetry aggregate: " .. message, 3)
end

local function counter(values, key)
	values[key] = (values[key] or 0) + 1
end

local function list(values, key)
	local result = {}
	for name, count in pairs(values) do
		table.insert(result, { [key] = name, count = count })
	end
	table.sort(result, function(left, right)
		return left[key] < right[key]
	end)
	return result
end

function M.new(opts)
	if opts == nil then
		opts = {}
	end
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	for key in pairs(opts) do
		if key ~= "consent" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	local gate = opts.consent or consent
	if type(gate) ~= "table" or type(gate.require) ~= "function" then
		fail("consent must expose require")
	end
	local require_consent
	if gate == consent then
		require_consent = consent.require
	else
		require_consent = function(action)
			return gate:require(action)
		end
	end
	return setmetatable({
		require_consent = require_consent,
		features = {},
		errors = {},
		performance = {},
		health = {},
	}, Aggregator)
end

function Aggregator:record(attrs)
	self.require_consent("collection")
	local value = event.event(attrs)
	if value.type == "feature" then
		counter(self.features, value.fields.feature)
	elseif value.type == "error" then
		counter(self.errors, value.fields.code)
	elseif value.type == "performance" then
		local entry = self.performance[value.fields.operation] or { count = 0, duration_ms = 0 }
		entry.count = entry.count + 1
		entry.duration_ms = entry.duration_ms + value.fields.duration_ms
		self.performance[value.fields.operation] = entry
	else
		counter(self.health, value.fields.component .. "." .. value.fields.status)
	end
	return true
end

function Aggregator:snapshot()
	local performance = {}
	for operation, value in pairs(self.performance) do
		table.insert(performance, { operation = operation, count = value.count, duration_ms = value.duration_ms })
	end
	table.sort(performance, function(left, right)
		return left.operation < right.operation
	end)
	local health = {}
	for key, count in pairs(self.health) do
		local component, status = key:match("^(.+)%.([^.]+)$")
		table.insert(health, { component = component, status = status, count = count })
	end
	table.sort(health, function(left, right)
		return left.component == right.component and left.status < right.status or left.component < right.component
	end)
	return {
		schema_version = M.schema_version,
		features = list(self.features, "feature"),
		errors = list(self.errors, "code"),
		performance = performance,
		health = health,
	}
end

return M
