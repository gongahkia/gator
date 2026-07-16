local M = {}

local function fail(message)
	error("Gator adapter authentication: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function status(value)
	if type(value) ~= "table" then
		fail("probe result must be a table")
	end
	for key in pairs(value) do
		if key ~= "authenticated" and key ~= "reason" then
			fail("probe result contains unsupported field: " .. tostring(key))
		end
	end
	if type(value.authenticated) ~= "boolean" then
		fail("probe result must declare authentication")
	end
	if value.authenticated and value.reason ~= nil then
		fail("authenticated probe result must not include a reason")
	end
	if not value.authenticated and (type(value.reason) ~= "string" or value.reason == "") then
		fail("unauthenticated probe result must include a reason")
	end
	return value.authenticated and { authenticated = true } or { authenticated = false, reason = value.reason }
end

function M.discover(opts)
	if type(opts) ~= "table" or type(opts.probe) ~= "function" then
		fail("discover requires a probe function")
	end
	for key in pairs(opts) do
		if key ~= "provider" and key ~= "probe" then
			fail("discover contains unsupported field: " .. tostring(key))
		end
	end
	local provider = identifier(opts.provider, "provider")
	local ok, result = pcall(opts.probe)
	if not ok then
		return { provider = provider, authenticated = false, reason = "CLI authentication probe failed" }
	end
	result = status(result)
	result.provider = provider
	return result
end

return M
