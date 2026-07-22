local pack = require("gator.context.pack")
local M = { schema_version = 1 }
local HandoffPack = {}

HandoffPack.__index = HandoffPack

local function fail(message)
	error("Gator handoff pack: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function entries(value)
	if type(value) ~= "table" or not vim.islist(value) then
		fail("entries must be an ordered array")
	end
	local result = {}
	for index, entry in ipairs(value) do
		result[index] = pack.entry(entry)
	end
	return result
end

local function attrs(value)
	if type(value) ~= "table" then
		fail("attributes must be a table")
	end
	for key in pairs(value) do
		if key ~= "schema_version" and key ~= "id" and key ~= "task_id" and key ~= "entries" then
			fail("attributes contain unsupported field: " .. tostring(key))
		end
	end
	if value.schema_version ~= nil and value.schema_version ~= M.schema_version then
		fail("schema_version is unsupported")
	end
	return {
		id = identifier(value.id, "id"),
		task_id = identifier(value.task_id, "task_id"),
		entries = entries(value.entries),
	}
end

function M.new(value)
	local value = attrs(value)
	return setmetatable(value, HandoffPack)
end

function M.is(value)
	return getmetatable(value) == HandoffPack
end

function M.to_record(value)
	if not M.is(value) then
		fail("value must be created by gator.context.handoff_pack.new")
	end
	local canonical = attrs(value)
	return {
		schema_version = M.schema_version,
		id = canonical.id,
		task_id = canonical.task_id,
		entries = canonical.entries,
	}
end

function M.from_record(value)
	return M.new(value)
end

return M
