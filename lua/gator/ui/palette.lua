local M = {}
local entries = {}
local kinds = { action = true, adapter = true, task = true, provider = true }

local function fail(message)
	error("Gator palette: " .. message, 3)
end

local function identifier(value)
	if type(value) ~= "string" or value == "" then
		fail("identifier must be a non-empty string")
	end
	return value
end

function M.register(value)
	if type(value) ~= "table" then
		fail("entry must be a table")
	end
	for key in pairs(value) do
		if key ~= "kind" and key ~= "name" and key ~= "execute" then
			fail("entry contains unsupported field: " .. tostring(key))
		end
	end
	local kind = identifier(value.kind)
	if not kinds[kind] then
		fail("entry kind is unknown: " .. kind)
	end
	local name = identifier(value.name)
	if not name:match("^[a-z][a-z0-9_-]*$") then
		fail("entry name must be a lowercase identifier")
	end
	if type(value.execute) ~= "function" then
		fail("entry execute must be a function")
	end
	local id = kind .. ":" .. name
	if entries[id] then
		fail("entry is already registered: " .. id)
	end
	entries[id] = { kind = kind, name = name, execute = value.execute }
	return id
end

function M.unregister(id)
	id = identifier(id)
	if not entries[id] then
		return false
	end
	entries[id] = nil
	return true
end

function M.complete(arglead)
	if type(arglead) ~= "string" then
		fail("completion prefix must be a string")
	end
	local result = {}
	for id in pairs(entries) do
		if id:sub(1, #arglead) == arglead then
			table.insert(result, id)
		end
	end
	table.sort(result)
	return result
end

function M.execute(id)
	id = identifier(id)
	local entry = entries[id]
	if not entry then
		fail("entry is not registered: " .. id)
	end
	return entry.execute()
end

function M.list()
	local result = {}
	for _, id in ipairs(M.complete("")) do
		local entry = entries[id]
		table.insert(result, { id = id, kind = entry.kind, name = entry.name })
	end
	return result
end

return M
