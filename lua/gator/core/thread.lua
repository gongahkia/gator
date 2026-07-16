local errors = require("gator.error")
local M = {}
local Thread = {}
local Store = {}

Thread.__index = Thread
Store.__index = Store

M.roles = { user = true, assistant = true, system = true, tool = true }

local function fail(detail)
	errors.raise(errors.new("thread.invalid", "Gator-owned thread is invalid", {
		detail = detail,
		remedy = "Store normalized Gator messages with explicit provider provenance, not native provider history.",
	}))
end

local function require_string(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty string")
	end
	return value
end

local function require_identifier(value, name)
	value = require_string(value, name)
	if not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function require_timestamp(value, name)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail(name .. " must be a non-negative integer timestamp")
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

local function provenance(value)
	if type(value) ~= "table" then
		fail("provenance must be a table")
	end
	if value.kind == "gator" then
		validate_fields(value, { kind = true }, "gator provenance")
		return { kind = "gator" }
	end
	if value.kind == "provider" then
		validate_fields(value, { kind = true, provider = true, session_id = true }, "provider provenance")
		return {
			kind = "provider",
			provider = require_string(value.provider, "provenance.provider"),
			session_id = require_string(value.session_id, "provenance.session_id"),
		}
	end
	fail("provenance.kind must be gator or provider")
end

function M.entry(attrs)
	if type(attrs) ~= "table" then
		fail("entry attributes must be a table")
	end
	validate_fields(attrs, { id = true, role = true, content = true, created_at = true, provenance = true }, "entry")
	local role = require_string(attrs.role, "entry.role")
	if not M.roles[role] then
		fail("entry.role is unknown: " .. role)
	end
	return {
		id = require_identifier(attrs.id, "entry.id"),
		role = role,
		content = require_string(attrs.content, "entry.content"),
		created_at = require_timestamp(attrs.created_at, "entry.created_at"),
		provenance = provenance(attrs.provenance),
	}
end

local function entries(value)
	if type(value) ~= "table" or not vim.islist(value) then
		fail("entries must be an array")
	end
	local result = {}

	for index, entry in ipairs(value) do
		result[index] = M.entry(entry)
	end
	return result
end

function M.new(attrs)
	if type(attrs) ~= "table" then
		fail("attributes must be a table")
	end
	validate_fields(
		attrs,
		{ id = true, task_id = true, entries = true, created_at = true, updated_at = true },
		"thread"
	)
	local created_at = attrs.created_at or os.time()
	local updated_at = attrs.updated_at or created_at
	require_timestamp(created_at, "created_at")
	require_timestamp(updated_at, "updated_at")
	if updated_at < created_at then
		fail("updated_at cannot precede created_at")
	end

	return setmetatable({
		id = require_identifier(attrs.id, "id"),
		task_id = require_identifier(attrs.task_id, "task_id"),
		entries = entries(attrs.entries or {}),
		created_at = created_at,
		updated_at = updated_at,
	}, Thread)
end

function M.is(value)
	return getmetatable(value) == Thread
end

function M.to_record(value)
	if not M.is(value) then
		fail("value must be created by gator.core.thread.new")
	end
	local thread = M.new(value)
	return {
		id = thread.id,
		task_id = thread.task_id,
		entries = entries(thread.entries),
		created_at = thread.created_at,
		updated_at = thread.updated_at,
	}
end

function M.from_record(record)
	return M.new(record)
end

function M.append(value, entry, updated_at)
	if not M.is(value) then
		fail("thread must be created by gator.core.thread.new")
	end
	updated_at = updated_at or os.time()
	if type(updated_at) ~= "number" or updated_at % 1 ~= 0 or updated_at < value.updated_at then
		fail("updated_at must be an integer no earlier than the current thread timestamp")
	end
	local record = M.to_record(value)
	table.insert(record.entries, M.entry(entry))
	record.updated_at = updated_at
	return M.from_record(record)
end

local function records(path)
	if vim.fn.filereadable(path) == 0 then
		return {}
	end
	local content = table.concat(vim.fn.readfile(path), "\n")
	local ok, document = pcall(vim.json.decode, content)
	if not ok or type(document) ~= "table" then
		fail("thread file is not valid JSON: " .. path)
	end
	if document.schema_version ~= 1 or type(document.threads) ~= "table" or not vim.islist(document.threads) then
		fail("thread file has an unsupported schema: " .. path)
	end
	return document.threads
end

local function write(path, values)
	local parent = vim.fn.fnamemodify(path, ":h")
	if vim.fn.mkdir(parent, "p") < 0 then
		fail("cannot create thread directory: " .. parent)
	end
	local temporary = path .. ".tmp-" .. vim.uv.hrtime()
	local ok, err = pcall(vim.fn.writefile, { vim.json.encode({ schema_version = 1, threads = values }) }, temporary)
	if not ok then
		fail("cannot write thread file: " .. err)
	end
	local renamed, rename_err = vim.uv.fs_rename(temporary, path)
	if not renamed then
		vim.fn.delete(temporary)
		fail("cannot replace thread file: " .. rename_err)
	end
end

function M.open(path)
	path = path or vim.fn.stdpath("state") .. "/gator/threads.json"
	if type(path) ~= "string" or path == "" then
		fail("path must be a non-empty string")
	end
	return setmetatable({ path = path }, Store)
end

function Store:put(value)
	local thread = M.to_record(value)
	local values = records(self.path)
	local replaced = false

	for index, record in ipairs(values) do
		if M.from_record(record).id == thread.id then
			values[index] = thread
			replaced = true
		end
	end
	if not replaced then
		table.insert(values, thread)
	end
	table.sort(values, function(left, right)
		return M.from_record(left).id < M.from_record(right).id
	end)
	write(self.path, values)
	return M.from_record(thread)
end

function Store:get(id)
	id = require_identifier(id, "id")
	for _, record in ipairs(records(self.path)) do
		local thread = M.from_record(record)
		if thread.id == id then
			return thread
		end
	end
	return nil
end

function Store:list(task_id)
	if task_id ~= nil then
		require_identifier(task_id, "task_id")
	end
	local result = {}
	for _, record in ipairs(records(self.path)) do
		local thread = M.from_record(record)
		if task_id == nil or thread.task_id == task_id then
			table.insert(result, thread)
		end
	end
	return result
end

return M
