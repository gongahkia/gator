local accessibility = require("gator.ui.accessibility")
local panel_window = require("gator.ui.window")

local M = {}
local panels = {}

local function fail(message)
	error("Gator retention UI: " .. message, 3)
end

local function current()
	local tabpage = vim.api.nvim_get_current_tabpage()
	local panel = panels[tabpage]
	if panel and vim.api.nvim_win_is_valid(panel.window) then
		return panel, tabpage
	end
	panels[tabpage] = nil
	return nil, tabpage
end

local function render(panel)
	local lines = { "Gator cleanup preview", "" }
	if #panel.artifacts == 0 and #panel.worktrees == 0 then
		table.insert(lines, "No expired Gator-owned artifacts or eligible worktrees.")
	else
		if #panel.artifacts > 0 then
			table.insert(lines, "Artifacts to delete: " .. #panel.artifacts)
			for _, value in ipairs(panel.artifacts) do
				table.insert(lines, "- " .. value.category .. " · " .. vim.fn.fnamemodify(value.path, ":~:."))
			end
		end
		if #panel.worktrees > 0 then
			table.insert(lines, "")
			table.insert(lines, "Clean inactive worktrees to remove: " .. #panel.worktrees)
			for _, value in ipairs(panel.worktrees) do
				table.insert(lines, "- " .. vim.fn.fnamemodify(value.path, ":~:."))
			end
		end
	end
	table.insert(lines, "")
	table.insert(lines, "<CR> delete shown targets · q cancel · ? help")
	accessibility.render(panel.buffer, lines, "gator-retention")
end

function M.open(opts)
	if
		type(opts) ~= "table"
		or type(opts.artifacts) ~= "table"
		or not vim.islist(opts.artifacts)
		or type(opts.worktrees) ~= "table"
		or not vim.islist(opts.worktrees)
		or type(opts.on_confirm) ~= "function"
	then
		fail("open requires artifact/worktree arrays and a confirm callback")
	end
	local panel, tabpage = current()
	if panel then
		panel.artifacts, panel.worktrees, panel.on_confirm =
			vim.deepcopy(opts.artifacts), vim.deepcopy(opts.worktrees), opts.on_confirm
		render(panel)
		vim.api.nvim_set_current_win(panel.window)
		return panel.window
	end
	local opened = panel_window.open("botright 18new")
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype, vim.bo[buffer].bufhidden = "gator-retention", "wipe"
	vim.api.nvim_win_set_buf(opened.window, buffer)
	panel = {
		window = opened.window,
		buffer = buffer,
		previous = opened.previous,
		artifacts = vim.deepcopy(opts.artifacts),
		worktrees = vim.deepcopy(opts.worktrees),
		on_confirm = opts.on_confirm,
	}
	panels[tabpage] = panel
	render(panel)
	accessibility.panel(buffer, { confirm = "<CR>", close = "q", help = "?" }, {
		confirm = function()
			panel.on_confirm()
			M.close()
		end,
		close = M.close,
		help = function()
			vim.notify(
				"Gator cleanup: <CR> deletes only listed Gator-owned artifacts and clean inactive worktrees",
				vim.log.levels.INFO
			)
		end,
	})
	return panel.window
end

function M.close()
	local panel, tabpage = current()
	if not panel then
		return false
	end
	panel_window.close(panel.window, panel.previous)
	panels[tabpage] = nil
	return true
end

return M
