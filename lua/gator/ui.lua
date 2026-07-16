local M = { name = "ui", api_version = 1, sidebar = require("gator.ui.sidebar") }
local panels = {}

local function fail(message)
	error("Gator UI: " .. message, 3)
end

local function current_panel()
	local tabpage = vim.api.nvim_get_current_tabpage()
	local panel = panels[tabpage]
	if panel and vim.api.nvim_win_is_valid(panel.window) then
		return panel, tabpage
	end
	panels[tabpage] = nil
	return nil, tabpage
end

local function height()
	return math.max(8, math.min(20, math.floor(vim.o.lines * 0.33)))
end

local function render(buffer, state)
	vim.api.nvim_buf_set_lines(buffer, 0, -1, false, {
		"Gator",
		"Foundation workspace is active.",
		"Adapter, context, task, and review panels are issue-tracked.",
		"Configured context mode: " .. state.config.context.mode,
	})
end

function M.open(state)
	if type(state) ~= "table" or type(state.config) ~= "table" or type(state.config.context) ~= "table" then
		fail("open requires initialized Gator state")
	end
	local panel, tabpage = current_panel()
	if panel then
		render(panel.buffer, state)
		vim.api.nvim_set_current_win(panel.window)
		return panel.window
	end
	local previous = vim.api.nvim_get_current_win()
	vim.cmd("botright " .. height() .. "new")
	local window = vim.api.nvim_get_current_win()
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype = "gator"
	vim.bo[buffer].bufhidden = "wipe"
	vim.api.nvim_win_set_buf(window, buffer)
	render(buffer, state)
	panels[tabpage] = { window = window, buffer = buffer, previous = previous }
	return window
end

function M.focus()
	local panel = current_panel()
	if not panel then
		fail("no Gator panel is open in this tab")
	end
	vim.api.nvim_set_current_win(panel.window)
	return panel.window
end

function M.resize(lines)
	local panel = current_panel()
	if not panel then
		fail("no Gator panel is open in this tab")
	end
	if type(lines) ~= "number" or lines < 1 or lines % 1 ~= 0 then
		fail("panel height must be a positive integer")
	end
	vim.api.nvim_win_set_height(panel.window, lines)
	return vim.api.nvim_win_get_height(panel.window)
end

function M.close()
	local panel, tabpage = current_panel()
	if not panel then
		return false
	end
	local previous = panel.previous
	vim.api.nvim_win_close(panel.window, true)
	panels[tabpage] = nil
	if previous and vim.api.nvim_win_is_valid(previous) then
		vim.api.nvim_set_current_win(previous)
	end
	return true
end

function M.restore(state)
	local panel = current_panel()
	if panel then
		return M.focus()
	end
	return M.open(state)
end

return M
