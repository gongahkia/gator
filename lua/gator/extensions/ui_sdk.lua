local palette = require("gator.ui.palette")
local M = { api_version = 1 }
local panels = {}

local function fail(message)
	error("Gator UI SDK: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

function M.register(attrs)
	if type(attrs) ~= "table" or type(attrs.render) ~= "function" then
		fail("register requires a panel renderer")
	end
	for key in pairs(attrs) do
		if key ~= "name" and key ~= "render" then
			fail("panel contains unsupported field: " .. tostring(key))
		end
	end
	local name = identifier(attrs.name, "panel name")
	if panels[name] then
		fail("panel is already registered: " .. name)
	end
	panels[name] = attrs.render
	return name
end

function M.unregister(name)
	name = identifier(name, "panel name")
	if not panels[name] then
		return false
	end
	panels[name] = nil
	return true
end

function M.open(name, context)
	name = identifier(name, "panel name")
	local render = panels[name]
	if not render then
		fail("panel is not registered: " .. name)
	end
	local ok, lines = pcall(render, vim.deepcopy(context or {}))
	if not ok or type(lines) ~= "table" or not vim.islist(lines) then
		fail("panel renderer must return a line list")
	end
	for index, line in ipairs(lines) do
		if type(line) ~= "string" then
			fail("panel line " .. index .. " must be a string")
		end
	end
	vim.cmd("botright 12new")
	local window = vim.api.nvim_get_current_win()
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].bufhidden = "wipe"
	vim.bo[buffer].filetype = "gator-extension"
	vim.api.nvim_win_set_buf(window, buffer)
	vim.api.nvim_buf_set_lines(buffer, 0, -1, false, lines)
	return window
end

function M.action(attrs)
	if type(attrs) ~= "table" then
		fail("action requires attributes")
	end
	return palette.register({ kind = "action", name = attrs.name, execute = attrs.execute })
end

return M
