local pack = require("gator.context.pack")
local task = require("gator.core.task")
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

local function task_entry(value)
	if not task.is(value) then
		fail("task must be created by gator.core.task.new")
	end
	local record = task.to_record(value)
	return pack.entry({
		id = "task-" .. record.id,
		kind = "task",
		ref = "gator-task://" .. record.id,
		provenance = { source = "task", ref = record.id },
		trust = "manual",
		token_estimate = { status = "unavailable", reason = "task content has not been provider-counted" },
		transfer = { eligible = true },
		content = record.objective,
	})
end

local function source_entries(value, name)
	if value == nil then
		return {}
	end
	if type(value) ~= "table" or not vim.islist(value) then
		fail(name .. " must be an ordered array")
	end
	local result = {}
	for index, source in ipairs(value) do
		if type(source) ~= "table" then
			fail(name .. "[" .. index .. "] must be a context entry or capture record")
		end
		result[index] = pack.entry(source.entry or source)
	end
	return result
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

function M.build(opts)
	if type(opts) ~= "table" then
		fail("build requires options")
	end
	for key in pairs(opts) do
		if
			key ~= "id"
			and key ~= "task"
			and key ~= "files"
			and key ~= "diffs"
			and key ~= "diagnostics"
			and key ~= "instructions"
		then
			fail("build options contain unsupported field: " .. tostring(key))
		end
	end
	if not task.is(opts.task) then
		fail("build requires a task created by gator.core.task.new")
	end
	local result = { task_entry(opts.task) }
	for _, name in ipairs({ "files", "diffs", "diagnostics", "instructions" }) do
		for _, entry in ipairs(source_entries(opts[name], name)) do
			table.insert(result, entry)
		end
	end
	return M.new({ id = opts.id, task_id = opts.task.id, entries = result })
end

return M
