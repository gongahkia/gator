local redact = require("gator.policy.redact")
local M = { api_version = 1, schema_version = 1 }
local Logger = {}

Logger.__index = Logger

local levels = { debug = true, info = true, warn = true, error = true }

local function fail(message)
	error("Gator diagnostic log: " .. message, 3)
end

local function fields(value, allowed, name)
	if type(value) ~= "table" then
		fail(name .. " must be a table")
	end
	for key in pairs(value) do
		if not allowed[key] then
			fail(name .. " contains unsupported field: " .. tostring(key))
		end
	end
end

local function positive(value, name)
	if type(value) ~= "number" or value < 1 or value % 1 ~= 0 then
		fail(name .. " must be a positive integer")
	end
	return value
end

local function task_id(value)
	if value == nil then
		return nil
	end
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail("task_id must be a lowercase identifier")
	end
	return value
end

local function timestamp(value)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail("at must be a non-negative integer timestamp")
	end
	return value
end

local function session_free(value, path)
	if type(value) ~= "table" then
		return
	end
	if value.provider ~= nil and value.id ~= nil and value.owner ~= nil then
		fail(path .. " must not include provider-owned session records")
	end
	for key, item in pairs(value) do
		session_free(item, path .. "." .. tostring(key))
	end
end

local function readable(path)
	return vim.uv.fs_stat(path) ~= nil
end

local function move(source, target)
	local ok, err = vim.uv.fs_rename(source, target)
	if not ok then
		fail("cannot rotate " .. source .. ": " .. tostring(err))
	end
end

function M.open(opts)
	fields(opts, { path = true, max_bytes = true, max_files = true }, "open")
	if type(opts.path) ~= "string" or opts.path == "" or not opts.path:match("^/") then
		fail("path must be an absolute path")
	end
	local parent = vim.fn.fnamemodify(opts.path, ":h")
	if vim.fn.isdirectory(parent) ~= 1 then
		fail("log directory does not exist: " .. parent)
	end
	return setmetatable({
		path = opts.path,
		max_bytes = positive(opts.max_bytes or 1024 * 1024, "max_bytes"),
		max_files = positive(opts.max_files or 5, "max_files"),
	}, Logger)
end

function Logger:rotate()
	if not readable(self.path) then
		return
	end
	if self.max_files == 1 then
		local ok, err = vim.uv.fs_unlink(self.path)
		if not ok then
			fail("cannot rotate " .. self.path .. ": " .. tostring(err))
		end
		return
	end
	local oldest = self.path .. "." .. (self.max_files - 1)
	if readable(oldest) then
		local ok, err = vim.uv.fs_unlink(oldest)
		if not ok then
			fail("cannot remove rotated log " .. oldest .. ": " .. tostring(err))
		end
	end
	for index = self.max_files - 2, 1, -1 do
		local source = self.path .. "." .. index
		if readable(source) then
			move(source, self.path .. "." .. (index + 1))
		end
	end
	move(self.path, self.path .. ".1")
end

function Logger:write(attrs)
	fields(attrs, { level = true, message = true, task_id = true, at = true, fields = true }, "record")
	if type(attrs.level) ~= "string" or not levels[attrs.level] then
		fail("level is unsupported")
	end
	if type(attrs.message) ~= "string" or attrs.message == "" then
		fail("message must be a non-empty string")
	end
	local details = attrs.fields == nil and {} or attrs.fields
	session_free(details, "record fields")
	local record = {
		schema_version = M.schema_version,
		level = attrs.level,
		message = redact.text(attrs.message),
		fields = redact.value(details),
		at = timestamp(attrs.at or os.time()),
	}
	local id = task_id(attrs.task_id)
	if id then
		record.task_id = id
	end
	local line = vim.json.encode(record)
	local stat = vim.uv.fs_stat(self.path)
	if stat and stat.size > 0 and stat.size + #line + 1 > self.max_bytes then
		self:rotate()
	end
	local ok, result = pcall(vim.fn.writefile, { line }, self.path, "a")
	if not ok or result ~= 0 then
		fail("cannot write log: " .. self.path)
	end
	return vim.deepcopy(record)
end

function Logger:paths()
	local result = {}
	for index = 0, self.max_files - 1 do
		local path = index == 0 and self.path or self.path .. "." .. index
		if readable(path) then
			table.insert(result, path)
		end
	end
	return result
end

return M
