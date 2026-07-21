local redact = require("gator.policy.redact")

local M = {}
local Filesystem = {}

Filesystem.__index = Filesystem

local function fail(message)
	error("Gator filesystem: " .. redact.text(tostring(message)), 3)
end

local function path(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty path")
	end
	return value
end

local defaults = {
	readable = function(value)
		return vim.fn.filereadable(value) == 1
	end,
	read = function(value)
		return table.concat(vim.fn.readfile(value, "b"), "\n")
	end,
	mkdir = function(value)
		return vim.fn.mkdir(value, "p") >= 0
	end,
	write = function(value, content)
		return vim.fn.writefile({ content }, value) == 0
	end,
	rename = function(source, target)
		return vim.uv.fs_rename(source, target)
	end,
	remove = function(value)
		return vim.fn.delete(value) == 0
	end,
}

function M.new(opts)
	if opts == nil then
		opts = {}
	end
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	local handlers = {}
	for name, callback in pairs(defaults) do
		if opts[name] ~= nil and type(opts[name]) ~= "function" then
			fail(name .. " must be a function")
		end
		handlers[name] = opts[name] or callback
	end
	for name in pairs(opts) do
		if not defaults[name] then
			fail("new contains unsupported field: " .. tostring(name))
		end
	end
	return setmetatable({ handlers = handlers }, Filesystem)
end

function M.is(value)
	return getmetatable(value) == Filesystem
end

local function invoke(self, name, ...)
	if not M.is(self) then
		fail(name .. " requires a filesystem")
	end
	local ok, value, detail = pcall(self.handlers[name], ...)
	if not ok then
		fail(name .. " failed: " .. tostring(value))
	end
	return value, detail
end

function Filesystem:readable(value)
	value = path(value, "path")
	local result = invoke(self, "readable", value)
	if type(result) ~= "boolean" then
		fail("readable must return boolean")
	end
	return result
end

function Filesystem:read(value)
	value = path(value, "path")
	local result = invoke(self, "read", value)
	if type(result) ~= "string" then
		fail("read must return text")
	end
	return result
end

function Filesystem:mkdir(value)
	value = path(value, "directory")
	local result, detail = invoke(self, "mkdir", value)
	if type(result) ~= "boolean" then
		fail("mkdir must return boolean")
	end
	return result, detail
end

function Filesystem:write(value, content)
	value = path(value, "path")
	if type(content) ~= "string" then
		fail("write content must be text")
	end
	local result, detail = invoke(self, "write", value, content)
	if type(result) ~= "boolean" then
		fail("write must return boolean")
	end
	return result, detail
end

function Filesystem:rename(source, target)
	source, target = path(source, "source"), path(target, "target")
	local result, detail = invoke(self, "rename", source, target)
	if type(result) ~= "boolean" then
		fail("rename must return boolean")
	end
	return result, detail
end

function Filesystem:remove(value)
	value = path(value, "path")
	local result, detail = invoke(self, "remove", value)
	if type(result) ~= "boolean" then
		fail("remove must return boolean")
	end
	return result, detail
end

return M
