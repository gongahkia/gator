local consent = require("gator.telemetry.consent")
local M = { api_version = 1, schema_version = 1 }
local Transport = {}

Transport.__index = Transport

local function fail(message)
	error("Gator telemetry export: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_.-]*$") then
		fail(name .. " must be a lowercase dotted identifier")
	end
	return value
end

local function count(value, name)
	if type(value) ~= "number" or value < 1 or value % 1 ~= 0 then
		fail(name .. " must be a positive integer")
	end
	return value
end

local function fields(value, allowed, name)
	if type(value) ~= "table" or (vim.islist(value) and next(value) ~= nil) then
		fail(name .. " must be an object")
	end
	for key in pairs(value) do
		if not allowed[key] then
			fail(name .. " contains unsupported field: " .. tostring(key))
		end
	end
end

local function rows(value, name, allowed, normalize)
	if type(value) ~= "table" or not vim.islist(value) then
		fail(name .. " must be a list")
	end
	local result = {}
	for index, row in ipairs(value) do
		fields(row, allowed, name .. "[" .. index .. "]")
		result[index] = normalize(row)
	end
	return result
end

local function snapshot(value)
	fields(value, {
		schema_version = true,
		features = true,
		errors = true,
		performance = true,
		health = true,
	}, "snapshot")
	if value.schema_version ~= M.schema_version then
		fail("snapshot schema_version is unsupported")
	end
	return {
		schema_version = M.schema_version,
		features = rows(value.features, "features", { feature = true, count = true }, function(row)
			return { feature = identifier(row.feature, "feature"), count = count(row.count, "feature count") }
		end),
		errors = rows(value.errors, "errors", { code = true, count = true }, function(row)
			return { code = identifier(row.code, "error code"), count = count(row.count, "error count") }
		end),
		performance = rows(
			value.performance,
			"performance",
			{ operation = true, count = true, duration_ms = true },
			function(row)
				if type(row.duration_ms) ~= "number" or row.duration_ms < 0 or row.duration_ms % 1 ~= 0 then
					fail("performance duration_ms must be a non-negative integer")
				end
				return {
					operation = identifier(row.operation, "operation"),
					count = count(row.count, "performance count"),
					duration_ms = row.duration_ms,
				}
			end
		),
		health = rows(value.health, "health", { component = true, status = true, count = true }, function(row)
			if row.status ~= "ready" and row.status ~= "degraded" and row.status ~= "unavailable" then
				fail("health status is unsupported")
			end
			return {
				component = identifier(row.component, "component"),
				status = row.status,
				count = count(row.count, "health count"),
			}
		end),
	}
end

local function endpoint(value)
	if
		type(value) ~= "string"
		or not value:match("^https://[%w][%w%._%-]*")
		or value:find("@", 1, true)
		or value:find("?", 1, true)
		or value:find("%s")
	then
		fail("endpoint must be a credential-free HTTPS URL")
	end
	return value
end

local function default_send(request)
	local value = vim.system({
		"curl",
		"--fail",
		"--silent",
		"--show-error",
		"--connect-timeout",
		"10",
		"--request",
		"POST",
		"--header",
		"Content-Type: application/json",
		"--data-binary",
		request.body,
		request.endpoint,
	}, { text = true }):wait()
	return { code = value.code }
end

function M.open(opts)
	if type(opts) ~= "table" then
		fail("open requires options")
	end
	for key in pairs(opts) do
		if key ~= "endpoint" and key ~= "consent" and key ~= "send" and key ~= "max_attempts" then
			fail("open contains unsupported field: " .. tostring(key))
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
	local attempts = opts.max_attempts == nil and 3 or count(opts.max_attempts, "max_attempts")
	if attempts > 5 then
		fail("max_attempts must not exceed 5")
	end
	if opts.send ~= nil and type(opts.send) ~= "function" then
		fail("send must be a function")
	end
	return setmetatable({
		endpoint = endpoint(opts.endpoint),
		require_consent = require_consent,
		request = opts.send or default_send,
		max_attempts = attempts,
		enabled = true,
		last = nil,
	}, Transport)
end

function Transport:disable()
	self.enabled = false
	return true
end

function Transport:inspect()
	return {
		enabled = self.enabled,
		endpoint = self.endpoint,
		max_attempts = self.max_attempts,
		last = self.last and vim.deepcopy(self.last) or nil,
	}
end

function Transport:send(value)
	if not self.enabled then
		fail("telemetry export is disabled")
	end
	self.require_consent("transmission")
	local body = vim.json.encode(snapshot(value))
	local result = { sent = false, attempts = 0, reason = "transport failed" }
	for attempt = 1, self.max_attempts do
		result.attempts = attempt
		local ok, response = pcall(self.request, { endpoint = self.endpoint, body = body })
		if
			ok
			and type(response) == "table"
			and type(response.code) == "number"
			and response.code >= 200
			and response.code < 300
		then
			result = { sent = true, attempts = attempt }
			break
		end
	end
	self.last = result
	return vim.deepcopy(result)
end

return M
