local redact = require("gator.policy.redact")
local schemas = {
	feature = { feature = true },
	error = { code = true, detail = true },
	performance = { operation = true, duration_ms = true },
	health = { component = true, status = true },
}
local M = { api_version = 1, schema_version = 1, schemas = vim.deepcopy(schemas) }

local function fail(message)
	error("Gator telemetry event: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_.-]*$") then
		fail(name .. " must be a lowercase dotted identifier")
	end
	return value
end

local function timestamp(value)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail("at must be a non-negative integer timestamp")
	end
	return value
end

function M.text(value)
	if type(value) ~= "string" then
		fail("text must be a string")
	end
	local result = redact.text(value)
	result = result:gsub("[A-Za-z]:[\\/][^%s,;\"'}]+", "[REDACTED_PATH]")
	result = result:gsub("~[/\\][^%s,;\"'}]+", "[REDACTED_PATH]")
	return result:gsub("/[%w%._%-][%w%._%-/]*", "[REDACTED_PATH]")
end

local function fields(kind, value)
	if type(value) ~= "table" or (vim.islist(value) and next(value) ~= nil) then
		fail("event fields must be an object")
	end
	local allowed = schemas[kind]
	for key in pairs(value) do
		if not allowed[key] then
			fail("event fields contain unsupported field: " .. tostring(key))
		end
	end
	if kind == "feature" then
		return { feature = identifier(value.feature, "feature") }
	end
	if kind == "error" then
		local result = { code = identifier(value.code, "error code") }
		if value.detail ~= nil then
			if type(value.detail) ~= "string" or value.detail == "" then
				fail("error detail must be a non-empty string")
			end
			result.detail = M.text(value.detail)
		end
		return result
	end
	if kind == "performance" then
		if type(value.duration_ms) ~= "number" or value.duration_ms < 0 or value.duration_ms % 1 ~= 0 then
			fail("duration_ms must be a non-negative integer")
		end
		return { operation = identifier(value.operation, "operation"), duration_ms = value.duration_ms }
	end
	if value.status ~= "ready" and value.status ~= "degraded" and value.status ~= "unavailable" then
		fail("health status is unsupported")
	end
	return { component = identifier(value.component, "component"), status = value.status }
end

function M.event(attrs)
	if type(attrs) ~= "table" then
		fail("event must be a table")
	end
	for key in pairs(attrs) do
		if key ~= "schema_version" and key ~= "type" and key ~= "at" and key ~= "fields" then
			fail("event contains unsupported field: " .. tostring(key))
		end
	end
	if attrs.schema_version ~= M.schema_version then
		fail("event schema_version is unsupported")
	end
	if type(attrs.type) ~= "string" or not schemas[attrs.type] then
		fail("event type is unsupported")
	end
	return {
		schema_version = M.schema_version,
		type = attrs.type,
		at = timestamp(attrs.at),
		fields = fields(attrs.type, attrs.fields),
	}
end

return M
