local capabilities = require("gator.adapters.capabilities")
local fixtures = require("gator.adapters.fixtures")
local M = { api_version = 1, capabilities = capabilities, fixtures = fixtures }

local function fail(message)
	error("Gator adapter SDK: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function safe(value, path)
	if type(value) == "string" or type(value) == "number" or type(value) == "boolean" then
		return value
	end
	if type(value) ~= "table" then
		fail(path .. " must be JSON-compatible")
	end
	local result = {}
	if vim.islist(value) then
		for index, item in ipairs(value) do
			result[index] = safe(item, path .. "[" .. index .. "]")
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
		result[key] = safe(item, path .. "." .. key)
	end
	return result
end

function M.event(attrs)
	if type(attrs) ~= "table" then
		fail("event attributes must be a table")
	end
	for key in pairs(attrs) do
		if key ~= "provider" and key ~= "type" and key ~= "payload" then
			fail("event contains unsupported field: " .. tostring(key))
		end
	end
	return {
		provider = identifier(attrs.provider, "event provider"),
		type = identifier(attrs.type, "event type"),
		payload = safe(attrs.payload or {}, "event payload"),
	}
end

function M.define(attrs)
	if type(attrs) ~= "table" then
		fail("define requires adapter attributes")
	end
	for key in pairs(attrs) do
		if key ~= "name" and key ~= "capabilities" and key ~= "emit" then
			fail("adapter contains unsupported field: " .. tostring(key))
		end
	end
	local name = identifier(attrs.name, "adapter name")
	if not capabilities.is(attrs.capabilities) or attrs.capabilities.provider ~= name then
		fail("adapter capabilities must belong to the adapter")
	end
	if type(attrs.emit) ~= "function" then
		fail("adapter emit must be a function")
	end
	return {
		name = name,
		capabilities = capabilities.to_record(attrs.capabilities),
		emit = function(event)
			event = M.event(event)
			if event.provider ~= name then
				fail("event provider must match adapter")
			end
			return attrs.emit(event)
		end,
	}
end

return M
