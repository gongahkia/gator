local redact = require("gator.policy.redact")

local M = {
	api_version = 1,
	schema_version = 1,
	domains = {
		message = true,
		tool = true,
		usage = true,
		context = true,
		file = true,
		permission = true,
		run = true,
	},
}
local Event = {}

Event.__index = Event

local function fail(message)
	error("Gator provider event: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function timestamp(value, name)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail(name .. " must be a non-negative integer timestamp")
	end
	return value
end

local function event_type(value)
	if type(value) ~= "string" then
		fail("type must be a provider event type")
	end
	local domain, action = value:match("^([a-z][a-z0-9_-]*)%.([a-z][a-z0-9_-]*)$")
	if not domain or not action or not M.domains[domain] then
		fail("type has an unsupported domain")
	end
	return value
end

local function provider(value)
	if type(value) ~= "table" then
		fail("provider must be a table")
	end
	for key in pairs(value) do
		if key ~= "name" and key ~= "session_id" then
			fail("provider contains unsupported field: " .. tostring(key))
		end
	end
	local result = { name = identifier(value.name, "provider.name") }
	if value.session_id ~= nil then
		if type(value.session_id) ~= "string" or value.session_id == "" then
			fail("provider.session_id must be a non-empty opaque identifier")
		end
		result.session_id = value.session_id
	end
	return result
end

local function payload(value, path)
	local kind = type(value)
	if kind == "string" then
		return redact.text(value)
	end
	if kind == "number" or kind == "boolean" then
		return value
	end
	if kind ~= "table" then
		fail(path .. " must be JSON-compatible")
	end
	local result = {}
	if vim.islist(value) then
		for index, item in ipairs(value) do
			result[index] = payload(item, path .. "[" .. index .. "]")
		end
		return result
	end
	for key, item in pairs(value) do
		if type(key) ~= "string" then
			fail(path .. " keys must be strings")
		end
		local lower = key:lower()
		if lower:match("token") or lower:match("secret") or lower:match("credential") or lower:match("password") then
			fail(path .. " must not include credentials")
		end
		result[key] = payload(item, path .. "." .. key)
	end
	if result.owner ~= nil then
		if
			type(result.provider) ~= "string"
			or result.provider == ""
			or type(result.id) ~= "string"
			or result.id == ""
			or result.owner ~= "provider"
		then
			fail(path .. " session references must remain provider-owned")
		end
	end
	return redact.value(result)
end

function M.new(attrs)
	if type(attrs) ~= "table" then
		fail("attributes must be a table")
	end
	for key in pairs(attrs) do
		if
			key ~= "schema_version"
			and key ~= "id"
			and key ~= "run_id"
			and key ~= "provider"
			and key ~= "sequence"
			and key ~= "type"
			and key ~= "at"
			and key ~= "payload"
		then
			fail("attributes contain unsupported field: " .. tostring(key))
		end
	end
	if attrs.schema_version ~= M.schema_version then
		fail("schema_version is unsupported")
	end
	if type(attrs.sequence) ~= "number" or attrs.sequence < 0 or attrs.sequence % 1 ~= 0 then
		fail("sequence must be a non-negative integer")
	end
	return setmetatable({
		schema_version = M.schema_version,
		id = identifier(attrs.id, "id"),
		run_id = identifier(attrs.run_id, "run_id"),
		provider = provider(attrs.provider),
		sequence = attrs.sequence,
		type = event_type(attrs.type),
		at = timestamp(attrs.at, "at"),
		payload = payload(attrs.payload or {}, "payload"),
	}, Event)
end

function M.is(value)
	return getmetatable(value) == Event
end

function M.to_record(value)
	if not M.is(value) then
		fail("value must be created by gator.core.provider_event.new")
	end
	return {
		schema_version = M.schema_version,
		id = value.id,
		run_id = value.run_id,
		provider = vim.deepcopy(value.provider),
		sequence = value.sequence,
		type = value.type,
		at = value.at,
		payload = vim.deepcopy(value.payload),
	}
end

function M.from_record(value)
	return M.new(value)
end

return M
