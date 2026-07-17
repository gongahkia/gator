local M = {
	name = "ui",
	api_version = 1,
	context_inspector = require("gator.ui.context_inspector"),
	dashboard = require("gator.ui.dashboard"),
	diff_review = require("gator.ui.diff_review"),
	escalation = require("gator.ui.escalation"),
	accessibility = require("gator.ui.accessibility"),
	markdown = require("gator.ui.markdown"),
	palette = require("gator.ui.palette"),
	picker = require("gator.ui.picker"),
	selection = require("gator.ui.selection"),
	sidebar = require("gator.ui.sidebar"),
	timeline = require("gator.ui.timeline"),
	workspace_dashboard = require("gator.ui.workspace_dashboard"),
	motion = require("gator.ui.motion"),
}
local panels = {}
local accessibility = require("gator.ui.accessibility")

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

local function render(panel)
	local state = panel.state
	local lines = {
		"Gator workspace",
		"Tasks: no task selected",
		"Sessions: provider-native sessions are linked to tasks",
		"Context: " .. state.config.context.mode .. " / " .. state.config.context.trust,
		"Review: select a task to inspect local evidence",
		"",
		"Actions:",
		(panel.selected == 1 and "> " or "  ") .. "Run health check",
		(panel.selected == 2 and "> " or "  ") .. "Close workspace",
		"<CR> confirm · j/k navigate · q close",
	}
	if state.config.ui.screen_reader then
		accessibility.text(panel.buffer, lines)
	else
		vim.bo[panel.buffer].modifiable = true
		vim.api.nvim_buf_set_lines(panel.buffer, 0, -1, false, lines)
		vim.bo[panel.buffer].modifiable = false
		vim.bo[panel.buffer].filetype = "gator"
	end
end

local function bind(panel)
	local keys = vim.tbl_extend(
		"force",
		{ next = "j", previous = "k", confirm = "<CR>", cancel = "q" },
		panel.state.config.ui.keymaps
	)
	accessibility.bind(panel.buffer, keys, {
		next = function()
			panel.selected = panel.selected % 2 + 1
			render(panel)
		end,
		previous = function()
			panel.selected = panel.selected == 1 and 2 or panel.selected - 1
			render(panel)
		end,
		confirm = function()
			if panel.selected == 1 then
				vim.cmd("checkhealth gator")
			else
				M.close()
			end
		end,
		cancel = function()
			M.close()
		end,
	})
end

function M.open(state)
	if type(state) ~= "table" or type(state.config) ~= "table" or type(state.config.context) ~= "table" then
		fail("open requires initialized Gator state")
	end
	local panel, tabpage = current_panel()
	if panel then
		panel.state = state
		render(panel)
		vim.api.nvim_set_current_win(panel.window)
		return panel.window
	end
	local previous = vim.api.nvim_get_current_win()
	local target = height()
	vim.cmd("botright 1new")
	local window = vim.api.nvim_get_current_win()
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype = "gator"
	vim.bo[buffer].bufhidden = "wipe"
	vim.api.nvim_win_set_buf(window, buffer)
	vim.api.nvim_win_set_height(window, target)
	panel = { window = window, buffer = buffer, previous = previous, state = state, selected = 1 }
	panels[tabpage] = panel
	render(panel)
	bind(panel)
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
