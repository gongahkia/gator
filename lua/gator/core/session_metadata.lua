local errors = require("gator.error")
local M = {}
local Metadata = {}
local Store = {}

Metadata.__index = Metadata
Store.__index = Store

local function fail(detail)
	errors.raise(errors.new("session_metadata.invalid", "Provider-native session metadata is invalid", {
		detail = detail,
		remedy = "Persist only provider session IDs, versions, and capability provenance in local Gator state.",
	}))
end

local function require_string(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty string")
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

local function capabilities(value)
	if type(value) ~= "table" then
		fail("capabilities must be a table")
	end
	local result = {}

	for name, available in pairs(value) do
		if type(name) ~= "string" or type(available) ~= "boolean" then
			fail("capabilities must map strings to booleans")
		end
		result[name] = available
	end
	return result
end

local function records(path)
	if vim.fn.filereadable(path) == 0 then
		return {}
	end
	local content = table.concat(vim.fn.readfile(path), "\n")
	local ok, document = pcall(vim.json.decode, content)
	if not ok or type(document) ~= "table" then
		fail("metadata file is not valid JSON: " .. path)
	end
	if document.schema_version ~= 1 or type(document.sessions) ~= "table" or not vim.islist(document.sessions) then
		fail("metadata file has an unsupported schema: " .. path)
	end
	return document.sessions
end

local function metadata_key(value)
	return value.task_id .. "\0" .. value.provider .. "\0" .. value.id
end

local function write(path, values)
	local parent = vim.fn.fnamemodify(path, ":h")
	if vim.fn.mkdir(parent, "p") < 0 then
		fail("cannot create metadata directory: " .. parent)
	end
	local temporary = path .. ".tmp-" .. vim.uv.hrtime()
	local document = { schema_version = 1, sessions = values }
	local ok, err = pcall(vim.fn.writefile, { vim.json.encode(document) }, temporary)
	if not ok then
		fail("cannot write metadata file: " .. err)
	end
	local renamed, rename_err = vim.uv.fs_rename(temporary, path)
	if not renamed then
		vim.fn.delete(temporary)
		fail("cannot replace metadata file: " .. rename_err)
	end
end

function M.new(attrs)
	if type(attrs) ~= "table" then
		fail("attributes must be a table")
	end
	validate_fields(attrs, {
		task_id = true,
		provider = true,
		id = true,
		owner = true,
		provider_version = true,
		capabilities = true,
		probed_at = true,
	}, "metadata")
	local task_id = require_string(attrs.task_id, "task_id")
	if not task_id:match("^[a-z][a-z0-9_-]*$") then
		fail("task_id must be a lowercase identifier")
	end
	if attrs.owner ~= "provider" then
		fail("owner must be provider")
	end

	return setmetatable({
		task_id = task_id,
		provider = require_string(attrs.provider, "provider"),
		id = require_string(attrs.id, "id"),
		owner = attrs.owner,
		provider_version = require_string(attrs.provider_version, "provider_version"),
		capabilities = capabilities(attrs.capabilities),
		probed_at = require_timestamp(attrs.probed_at, "probed_at"),
	}, Metadata)
end

function M.is(value)
	return getmetatable(value) == Metadata
end

function M.to_record(value)
	if not M.is(value) then
		fail("value must be created by gator.core.session_metadata.new")
	end
	local metadata = M.new(value)
	return {
		task_id = metadata.task_id,
		provider = metadata.provider,
		id = metadata.id,
		owner = metadata.owner,
		provider_version = metadata.provider_version,
		capabilities = capabilities(metadata.capabilities),
		probed_at = metadata.probed_at,
	}
end

function M.from_record(record)
	return M.new(record)
end

function M.open(path)
	path = path or vim.fn.stdpath("state") .. "/gator/sessions.json"
	if type(path) ~= "string" or path == "" then
		fail("path must be a non-empty string")
	end
	return setmetatable({ path = path }, Store)
end

function Store:put(value)
	local metadata = M.to_record(value)
	local values = records(self.path)
	local key = metadata_key(metadata)
	local replaced = false

	for index, record in ipairs(values) do
		if metadata_key(M.from_record(record)) == key then
			values[index] = metadata
			replaced = true
		end
	end
	if not replaced then
		table.insert(values, metadata)
	end
	table.sort(values, function(left, right)
		return metadata_key(M.from_record(left)) < metadata_key(M.from_record(right))
	end)
	write(self.path, values)
	return M.from_record(metadata)
end

function Store:get(task_id, provider, id)
	local target = metadata_key({
		task_id = require_string(task_id, "task_id"),
		provider = require_string(provider, "provider"),
		id = require_string(id, "id"),
	})

	for _, record in ipairs(records(self.path)) do
		local metadata = M.from_record(record)
		if metadata_key(metadata) == target then
			return metadata
		end
	end
	return nil
end

function Store:list(task_id)
	if task_id ~= nil then
		require_string(task_id, "task_id")
	end
	local result = {}

	for _, record in ipairs(records(self.path)) do
		local metadata = M.from_record(record)
		if task_id == nil or metadata.task_id == task_id then
			table.insert(result, metadata)
		end
	end
	return result
end

return M
