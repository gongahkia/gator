local M = {}
local Contract = {}

Contract.__index = Contract

M.domains = {
	transport = true,
	auth = true,
	session = true,
	permission = true,
	model = true,
	command = true,
	tool = true,
	context = true,
	usage = true,
}

local function fail(message)
	error("Gator adapter capabilities: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function domain(value, name)
	if type(value) ~= "table" then
		fail(name .. " must be a table")
	end
	for key in pairs(value) do
		if key ~= "available" and key ~= "modes" and key ~= "reason" then
			fail(name .. " contains unsupported field: " .. tostring(key))
		end
	end
	if type(value.available) ~= "boolean" then
		fail(name .. " must declare availability")
	end
	if value.available then
		if type(value.modes) ~= "table" or not vim.islist(value.modes) or #value.modes == 0 or value.reason ~= nil then
			fail(name .. " availability requires modes and no reason")
		end
		local modes, seen = {}, {}
		for index, mode in ipairs(value.modes) do
			mode = identifier(mode, name .. " mode " .. index)
			if seen[mode] then
				fail(name .. " modes must be unique")
			end
			seen[mode] = true
			modes[index] = mode
		end
		return { available = true, modes = modes }
	end
	if value.modes ~= nil or type(value.reason) ~= "string" or value.reason == "" then
		fail(name .. " unavailability requires a reason and no modes")
	end
	return { available = false, reason = value.reason }
end

function M.new(attrs)
	if type(attrs) ~= "table" then
		fail("attributes must be a table")
	end
	for key in pairs(attrs) do
		if key ~= "provider" and not M.domains[key] then
			fail("attributes contain unsupported field: " .. tostring(key))
		end
	end
	local result = { provider = identifier(attrs.provider, "provider") }
	for name in pairs(M.domains) do
		result[name] = domain(attrs[name], name)
	end
	return setmetatable(result, Contract)
end

function M.is(value)
	return getmetatable(value) == Contract
end

function M.to_record(value)
	if not M.is(value) then
		fail("value must be created by gator.adapters.capabilities.new")
	end
	return M.new(value)
end

function M.from_record(value)
	return M.new(value)
end

function M.supports(value, name, mode)
	if not M.is(value) then
		fail("value must be created by gator.adapters.capabilities.new")
	end
	if not M.domains[name] then
		fail("capability domain is unknown: " .. tostring(name))
	end
	local capability = value[name]
	if not capability.available then
		return false, capability.reason
	end
	if mode == nil then
		return true
	end
	mode = identifier(mode, "capability mode")
	for _, advertised in ipairs(capability.modes) do
		if advertised == mode then
			return true
		end
	end
	return false, "mode is not advertised: " .. mode
end

function M.require(value, name, mode)
	local supported, reason = M.supports(value, name, mode)
	if not supported then
		fail(name .. " capability is unavailable: " .. reason)
	end
	return true
end

return M
