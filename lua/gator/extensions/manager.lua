local health = require("gator.health")
local actions = require("gator.ui.actions")
local M = {}
local Manager = {}

Manager.__index = Manager

local function fail(message)
	error("Gator extensions: " .. message, 3)
end

local function name(value, label)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(label .. " must be a lowercase identifier")
	end
	return value
end

local function sandbox()
	local value = {
		assert = assert,
		error = error,
		ipairs = ipairs,
		pairs = pairs,
		pcall = pcall,
		tostring = tostring,
		type = type,
		math = math,
		string = string,
		table = table,
	}
	value._G = value
	return value
end

local function manifest(value, path)
	if type(value) ~= "table" then
		fail("extension manifest must be a table: " .. path)
	end
	for key in pairs(value) do
		if key ~= "name" and key ~= "api_version" and key ~= "setup" then
			fail("extension manifest contains unsupported field: " .. tostring(key))
		end
	end
	if value.api_version ~= 1 then
		fail("extension API version is unsupported: " .. path)
	end
	if type(value.setup) ~= "function" then
		fail("extension setup must be a function: " .. path)
	end
	return { name = name(value.name, "extension name"), setup = value.setup, path = path }
end

local function load(path)
	local chunk, err = loadfile(path, "t", sandbox())
	if not chunk then
		fail("cannot load extension: " .. err)
	end
	local ok, value = xpcall(chunk, debug.traceback)
	if not ok then
		fail("extension manifest failed: " .. value)
	end
	return manifest(value, path)
end

local function remove(value)
	for _, id in ipairs(value.actions) do
		actions.unregister(id)
	end
	for _, check in ipairs(value.health) do
		health.unregister(check)
	end
end

function M.open(opts)
	if type(opts) ~= "table" then
		fail("open requires options")
	end
	for key in pairs(opts) do
		if key ~= "paths" then
			fail("open contains unsupported field: " .. tostring(key))
		end
	end
	if type(opts.paths) ~= "table" or not vim.islist(opts.paths) then
		fail("paths must be a list")
	end
	local paths = {}
	for index, path in ipairs(opts.paths) do
		local resolved = type(path) == "string" and vim.uv.fs_realpath(path) or nil
		if not resolved or vim.fn.isdirectory(resolved) ~= 1 then
			fail("paths[" .. index .. "] must be an existing directory")
		end
		paths[index] = resolved
	end
	return setmetatable({ paths = paths, manifests = {}, loaded = {} }, Manager)
end

function Manager:discover()
	local result = {}
	for _, directory in ipairs(self.paths) do
		for _, path in ipairs(vim.fn.glob(directory .. "/*.lua", false, true)) do
			local value = load(path)
			if result[value.name] or self.manifests[value.name] then
				fail("extension name is duplicated: " .. value.name)
			end
			result[value.name] = value
		end
	end
	self.manifests = result
	local names = vim.tbl_keys(result)
	table.sort(names)
	return names
end

function Manager:load(value)
	value = name(value, "extension name")
	if self.loaded[value] then
		fail("extension is already loaded: " .. value)
	end
	local extension = self.manifests[value]
	if not extension then
		fail("extension was not discovered: " .. value)
	end
	local registrations = { actions = {}, health = {} }
	local api = {
		actions = {
			register = function(entry)
				local id = actions.register(entry)
				table.insert(registrations.actions, id)
				return id
			end,
		},
		health = {
			register = function(check, callback)
				check = "extension." .. value .. "." .. name(check, "health check")
				health.register(check, callback)
				table.insert(registrations.health, check)
				return check
			end,
		},
	}
	local ok, cleanup = xpcall(function()
		return extension.setup(api)
	end, debug.traceback)
	if not ok or (cleanup ~= nil and type(cleanup) ~= "function") then
		remove(registrations)
		fail("extension setup failed: " .. tostring(cleanup))
	end
	self.loaded[value] =
		{ path = extension.path, cleanup = cleanup, actions = registrations.actions, health = registrations.health }
	return { name = value, path = extension.path }
end

function Manager:unload(value)
	value = name(value, "extension name")
	local extension = self.loaded[value]
	if not extension then
		return false
	end
	self.loaded[value] = nil
	remove(extension)
	if extension.cleanup then
		local ok, err = xpcall(extension.cleanup, debug.traceback)
		if not ok then
			fail("extension cleanup failed: " .. err)
		end
	end
	return true
end

function Manager:list()
	local result = {}
	for _, value in ipairs(vim.tbl_keys(self.manifests)) do
		local extension = self.manifests[value]
		table.insert(result, { name = value, path = extension.path, loaded = self.loaded[value] ~= nil })
	end
	return result
end

return M
