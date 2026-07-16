local errors = require("gator.error")
local M = {}
local Overlay = {}

Overlay.__index = Overlay

M.scopes = { global = true, project = true, repository = true, file = true, run = true }

local function fail(detail)
	errors.raise(errors.new("policy.invalid", "Policy overlay is invalid", {
		detail = detail,
		remedy = "Use a supported scope with explicit provenance and credential-free policy rules.",
	}))
end

local function require_string(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty string")
	end
	return value
end

local function validate_fields(value, allowed, name)
	for key in pairs(value) do
		if not allowed[key] then
			fail(name .. " contains unsupported field: " .. tostring(key))
		end
	end
end

local function safe_value(value, path)
	local kind = type(value)
	if kind == "string" or kind == "number" or kind == "boolean" then
		return value
	end
	if kind ~= "table" then
		fail(path .. " must be JSON-compatible")
	end
	local result = {}
	if vim.islist(value) then
		for index, child in ipairs(value) do
			result[index] = safe_value(child, path .. "[" .. index .. "]")
		end
		return result
	end
	for key, child in pairs(value) do
		if type(key) ~= "string" then
			fail(path .. " keys must be strings")
		end
		if
			key:lower():match("token")
			or key:lower():match("secret")
			or key:lower():match("credential")
			or key:lower():match("password")
		then
			fail(path .. " must not store credentials")
		end
		result[key] = safe_value(child, path .. "." .. key)
	end
	return result
end

local function provenance(value)
	if type(value) ~= "table" then
		fail("provenance must be a table")
	end
	validate_fields(value, { source = true, ref = true }, "provenance")
	return {
		source = require_string(value.source, "provenance.source"),
		ref = require_string(value.ref, "provenance.ref"),
	}
end

function M.new(attrs)
	if type(attrs) ~= "table" then
		fail("attributes must be a table")
	end
	validate_fields(attrs, { scope = true, target = true, rules = true, provenance = true }, "overlay")
	local scope = require_string(attrs.scope, "scope")
	if not M.scopes[scope] then
		fail("scope is unknown: " .. scope)
	end
	if scope == "global" then
		if attrs.target ~= nil then
			fail("global overlays must not declare a target")
		end
	else
		require_string(attrs.target, "target")
	end
	if type(attrs.rules) ~= "table" then
		fail("rules must be a table")
	end

	return setmetatable({
		scope = scope,
		target = attrs.target,
		rules = safe_value(attrs.rules, "rules"),
		provenance = provenance(attrs.provenance),
	}, Overlay)
end

function M.is(value)
	return getmetatable(value) == Overlay
end

function M.to_record(value)
	if not M.is(value) then
		fail("value must be created by gator.policy.overlay.new")
	end
	local overlay = M.new(value)
	return {
		scope = overlay.scope,
		target = overlay.target,
		rules = safe_value(overlay.rules, "rules"),
		provenance = provenance(overlay.provenance),
	}
end

function M.from_record(record)
	return M.new(record)
end

return M
