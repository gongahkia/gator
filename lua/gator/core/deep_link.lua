local capabilities = require("gator.adapters.capabilities")
local redact = require("gator.policy.redact")
local session = require("gator.core.session")

local M = { statuses = { available = true, unavailable = true, failed = true, cancelled = true } }

local function fail(message)
	error("Gator provider deep link: " .. redact.text(tostring(message)), 3)
end

local function unavailable(reason)
	return { status = "unavailable", reason = redact.text(reason) }
end

local function cancelled(value)
	if value == nil then
		return false
	end
	local ok, result = pcall(value)
	if not ok then
		fail("cancellation check failed")
	end
	if type(result) ~= "boolean" then
		fail("cancellation check must return a boolean")
	end
	return result
end

local function uri(value)
	if type(value) ~= "string" or value == "" then
		fail("native resolver must return a non-empty URI")
	end
	if value:find("[%c%s]") or not value:match("^[a-z][a-z0-9+.-]*:") then
		fail("native resolver must return an absolute URI")
	end
	if value:match("^[a-z][a-z0-9+.-]*://[^/]*@") then
		fail("native resolver URI must not contain user credentials")
	end
	for key in value:gmatch("[?&#]([^=&#]+)=") do
		local normalized = key:lower()
		if
			normalized:match("token")
			or normalized:match("secret")
			or normalized:match("credential")
			or normalized:match("password")
			or normalized:match("authorization")
			or normalized:match("api[_-]?key")
			or normalized:match("access[_-]?key")
		then
			fail("native resolver URI must not contain credentials")
		end
	end
	if redact.text(value) ~= value then
		fail("native resolver URI must not contain credentials")
	end
	return value
end

function M.resolve(opts)
	if type(opts) ~= "table" or not session.is(opts.session) or not capabilities.is(opts.capabilities) then
		fail("resolve requires a Gator session and capability contract")
	end
	for key in pairs(opts) do
		if key ~= "session" and key ~= "capabilities" and key ~= "native_resolve" and key ~= "cancelled" then
			fail("resolve contains unsupported field: " .. tostring(key))
		end
	end
	if opts.capabilities.provider ~= opts.session.provider then
		fail("capability provider must match the session provider")
	end
	if type(opts.native_resolve) ~= "function" then
		fail("resolve requires a native_resolve callback")
	end
	local supported, reason = capabilities.supports(opts.capabilities, "session", "deep_link")
	if not supported then
		return unavailable(reason)
	end
	if cancelled(opts.cancelled) then
		return { status = "cancelled" }
	end
	local reference = session.reference(opts.session)
	local ok, value = pcall(opts.native_resolve, vim.deepcopy(reference))
	if cancelled(opts.cancelled) then
		return { status = "cancelled" }
	end
	if not ok then
		return { status = "failed", reason = redact.text(tostring(value)) }
	end
	local valid, link = pcall(uri, value)
	if not valid then
		return { status = "failed", reason = redact.text(tostring(link)) }
	end
	return { status = "available", reference = reference, uri = link }
end

return M
