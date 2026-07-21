local capabilities = require("gator.adapters.capabilities")
local redact = require("gator.policy.redact")

local M = {
	api_version = 1,
	states = { ready = true, degraded = true, unavailable = true, failed = true, cancelled = true },
}

local function fail(message)
	error("Gator provider availability: " .. redact.text(tostring(message)), 3)
end

local function requirement(value, name)
	if type(value) ~= "table" then
		fail(name .. " must be an object")
	end
	for key in pairs(value) do
		if key ~= "domain" and key ~= "mode" then
			fail(name .. " contains unsupported field: " .. tostring(key))
		end
	end
	if type(value.domain) ~= "string" or not capabilities.domains[value.domain] then
		fail(name .. " domain is unavailable")
	end
	if type(value.mode) ~= "string" or not value.mode:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " mode must be a lowercase identifier")
	end
	return { domain = value.domain, mode = value.mode }
end

local function requirements(value, name)
	if type(value) ~= "table" or not vim.islist(value) then
		fail(name .. " must be an array")
	end
	local result, seen = {}, {}
	for index, item in ipairs(value) do
		local normalized = requirement(item, name .. " " .. index)
		local key = normalized.domain .. "\0" .. normalized.mode
		if seen[key] then
			fail(name .. " contains duplicate capabilities")
		end
		seen[key] = true
		result[index] = normalized
	end
	return result
end

local function status(contract, state, reason, degraded)
	return {
		provider = contract.provider,
		available = state == "ready" or state == "degraded",
		state = state,
		reason = reason and redact.text(reason) or nil,
		degraded = vim.deepcopy(degraded or {}),
	}
end

local function cancelled(callback, contract)
	if callback == nil then
		return nil
	end
	local ok, value = pcall(callback, contract.provider)
	if not ok or type(value) ~= "boolean" then
		return status(contract, "failed", ok and "cancel callback must return a boolean" or tostring(value))
	end
	if value then
		return status(contract, "cancelled", "provider availability evaluation was cancelled")
	end
	return nil
end

local function probe(callback, contract)
	if callback == nil then
		return nil
	end
	local ok, value = pcall(callback, contract.provider)
	if not ok then
		return status(contract, "failed", tostring(value))
	end
	if type(value) ~= "table" then
		return status(contract, "failed", "provider availability probe must return an object")
	end
	for key in pairs(value) do
		if key ~= "available" and key ~= "reason" then
			return status(
				contract,
				"failed",
				"provider availability probe returned unsupported field: " .. tostring(key)
			)
		end
	end
	if type(value.available) ~= "boolean" then
		return status(contract, "failed", "provider availability probe must return availability")
	end
	if value.reason ~= nil and (type(value.reason) ~= "string" or value.reason == "") then
		return status(contract, "failed", "provider availability probe reason must be non-empty text")
	end
	if not value.available then
		return status(contract, "unavailable", value.reason or "provider probe reported unavailable")
	end
	return nil
end

function M.assess(opts)
	if type(opts) ~= "table" or not capabilities.is(opts.capabilities) then
		fail("assess requires a capability contract")
	end
	for key in pairs(opts) do
		if key ~= "capabilities" and key ~= "required" and key ~= "optional" and key ~= "probe" and key ~= "cancel" then
			fail("assess contains unsupported field: " .. tostring(key))
		end
	end
	if opts.probe ~= nil and type(opts.probe) ~= "function" then
		fail("probe must be a function")
	end
	if opts.cancel ~= nil and type(opts.cancel) ~= "function" then
		fail("cancel must be a function")
	end
	local required = requirements(opts.required or {}, "required")
	local optional = requirements(opts.optional or {}, "optional")
	local stopped = cancelled(opts.cancel, opts.capabilities)
	if stopped then
		return stopped
	end
	local probed = probe(opts.probe, opts.capabilities)
	if probed then
		return probed
	end
	for _, value in ipairs(required) do
		local available, reason = capabilities.supports(opts.capabilities, value.domain, value.mode)
		if not available then
			return status(
				opts.capabilities,
				"unavailable",
				"required " .. value.domain .. "." .. value.mode .. " capability is unavailable: " .. reason
			)
		end
	end
	local degraded = {}
	for _, value in ipairs(optional) do
		local available, reason = capabilities.supports(opts.capabilities, value.domain, value.mode)
		if not available then
			table.insert(degraded, { domain = value.domain, mode = value.mode, reason = redact.text(reason) })
		end
	end
	if #degraded > 0 then
		return status(opts.capabilities, "degraded", nil, degraded)
	end
	return status(opts.capabilities, "ready")
end

return M
