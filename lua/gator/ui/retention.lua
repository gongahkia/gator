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

local function bytes(value)
	if value < 1024 then
		return value .. " B"
	end
	if value < 1024 * 1024 then
		return string.format("%.1f KiB", value / 1024)
	end
	return string.format("%.1f MiB", value / (1024 * 1024))
end

local function render(panel)
	local quota = panel.quota_bytes == 0 and "unbounded" or bytes(panel.quota_bytes)
	local lines = { panel.readonly and "Gator local storage" or "Gator cleanup preview", "" }
	table.insert(lines, "Stored: " .. bytes(panel.inventory.bytes) .. " · quota: " .. quota)
	for _, category in ipairs(panel.inventory.categories) do
		if category.files > 0 then
			table.insert(
				lines,
				"- " .. category.category .. " · " .. category.files .. " files · " .. bytes(category.bytes)
			)
		end
	end
	table.insert(lines, "")
	if #panel.artifacts == 0 and #panel.worktrees == 0 then
		table.insert(lines, "No Gator-owned artifacts are selected for deletion.")
	else
		if #panel.artifacts > 0 then
			table.insert(
				lines,
				"Artifacts to delete: " .. #panel.artifacts .. " · reclaim " .. bytes(panel.reclaim_bytes)
			)
			for _, value in ipairs(panel.artifacts) do
				table.insert(
					lines,
					"- "
						.. value.category
						.. " · "
						.. vim.fn.fnamemodify(value.path, ":~:.")
						.. " · "
						.. bytes(value.bytes or 0)
						.. " · "
						.. (value.reason or "age")
				)
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
	table.insert(lines, panel.readonly and "q close · ? help" or "<CR> delete shown targets · q cancel · ? help")
	accessibility.render(panel.buffer, lines, "gator-retention")
end

function M.open(opts)
	if
		type(opts) ~= "table"
		or type(opts.artifacts) ~= "table"
		or not vim.islist(opts.artifacts)
		or type(opts.worktrees) ~= "table"
		or not vim.islist(opts.worktrees)
	then
		fail("open requires artifact and worktree arrays")
	end
	if
		type(opts.inventory) ~= "table"
		or type(opts.inventory.bytes) ~= "number"
		or type(opts.inventory.categories) ~= "table"
		or not vim.islist(opts.inventory.categories)
	then
		fail("open requires a storage inventory")
	end
	if
		type(opts.quota_bytes) ~= "number"
		or opts.quota_bytes < 0
		or type(opts.reclaim_bytes) ~= "number"
		or opts.reclaim_bytes < 0
	then
		fail("open requires non-negative quota and reclaim bytes")
	end
	if opts.readonly ~= true and type(opts.on_confirm) ~= "function" then
		fail("cleanup preview requires a confirm callback")
	end
	local panel, tabpage = current()
	if panel then
		panel.artifacts, panel.worktrees, panel.inventory, panel.quota_bytes, panel.reclaim_bytes, panel.on_confirm, panel.readonly =
			vim.deepcopy(opts.artifacts),
			vim.deepcopy(opts.worktrees),
			vim.deepcopy(opts.inventory),
			opts.quota_bytes,
			opts.reclaim_bytes,
			opts.on_confirm,
			opts.readonly == true
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
		inventory = vim.deepcopy(opts.inventory),
		quota_bytes = opts.quota_bytes,
		reclaim_bytes = opts.reclaim_bytes,
		on_confirm = opts.on_confirm,
		readonly = opts.readonly == true,
	}
	panels[tabpage] = panel
	render(panel)
	accessibility.panel(buffer, { confirm = "<CR>", close = "q", help = "?" }, {
		confirm = function()
			if not panel.readonly then
				panel.on_confirm()
				M.close()
			end
		end,
		close = M.close,
		help = function()
			vim.notify(
				panel.readonly and "Gator storage: q closes this local inventory"
					or "Gator cleanup: <CR> deletes only listed Gator-owned artifacts and clean inactive worktrees",
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
