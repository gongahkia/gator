local M = {}
local entries = {}

local function fail(message)
	error("Gator workspace actions: " .. message, 3)
end

local function identifier(value)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail("name must be a lowercase identifier")
	end
	return value
end

function M.register(value)
	if type(value) ~= "table" then
		fail("action must be a table")
	end
	for key in pairs(value) do
		if key ~= "name" and key ~= "label" and key ~= "execute" then
			fail("action contains unsupported field: " .. tostring(key))
		end
	end
	local name = identifier(value.name)
	if value.label ~= nil and (type(value.label) ~= "string" or value.label == "") then
		fail("label must be non-empty text")
	end
	if type(value.execute) ~= "function" then
		fail("execute must be a function")
	end
	if entries[name] then
		fail("action is already registered: " .. name)
	end
	entries[name] = { name = name, label = value.label or name, execute = value.execute }
	return name
end

function M.unregister(name)
	name = identifier(name)
	if not entries[name] then
		return false
	end
	entries[name] = nil
	return true
end

function M.list()
	local result = {}
	for _, name in ipairs(vim.tbl_keys(entries)) do
		table.insert(result, vim.deepcopy(entries[name]))
	end
	table.sort(result, function(left, right)
		return left.name < right.name
	end)
	return result
end

return M
