local accessibility = require("gator.ui.accessibility")
local panel_window = require("gator.ui.window")

local M = {}
local panels = {}
local settings = { enabled = true }
local header_namespace = vim.api.nvim_create_namespace("GatorComposerHeader")
local attachment_namespace = vim.api.nvim_create_namespace("GatorComposerAttachments")

local function fail(message)
	error("Gator composer: " .. tostring(message), 3)
end

function M.configure(opts)
	if type(opts) ~= "table" or type(opts.enabled) ~= "boolean" then
		fail("settings require enabled boolean")
	end
	settings = vim.deepcopy(opts)
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

local function candidates(panel, trigger, query)
	local list = trigger == "#" and panel.candidates.skills or panel.candidates.files
	local result = {}
	for _, item in ipairs(list) do
		if query == "" or item.label:lower():find(query:lower(), 1, true) then
			table.insert(result, { word = item.label, abbr = item.label, menu = item.detail, icase = 1, dup = 0 })
		end
	end
	return result
end

local function completion(panel)
	local line = vim.api.nvim_get_current_line()
	local column = vim.fn.col(".") - 1
	local before = line:sub(1, column)
	local trigger, query = before:match("([#@])([^%s]*)$")
	if not trigger then
		return
	end
	local items = candidates(panel, trigger, query)
	if #items > 0 then
		vim.fn.complete(column - #query + 1, items)
	end
end

local function decorate(buffer)
	vim.api.nvim_set_hl(0, "GatorComposerTitle", { default = true, link = "Title" })
	vim.api.nvim_set_hl(0, "GatorComposerHint", { default = true, link = "Comment" })
	vim.api.nvim_set_hl(0, "GatorComposerAttachment", { default = true, link = "Special" })
	vim.api.nvim_buf_clear_namespace(buffer, header_namespace, 0, -1)
	vim.api.nvim_buf_set_extmark(buffer, header_namespace, 0, 0, {
		virt_lines = {
			{ { " Gator", "GatorComposerTitle" } },
			{ { " # skill | @ tracked file | Ctrl-S send | Esc cancel", "GatorComposerHint" } },
		},
		virt_lines_above = true,
	})
end

local function highlight_attachments(buffer)
	vim.api.nvim_buf_clear_namespace(buffer, attachment_namespace, 0, -1)
	for row, line in ipairs(vim.api.nvim_buf_get_lines(buffer, 0, -1, false)) do
		local start = 1
		while true do
			local first, last = line:find("[#@][%w_.%-/]+", start)
			if not first then
				break
			end
			vim.api.nvim_buf_add_highlight(
				buffer,
				attachment_namespace,
				"GatorComposerAttachment",
				row - 1,
				first - 1,
				last
			)
			start = last + 1
		end
	end
end

function M.open(opts)
	if type(opts) ~= "table" or type(opts.on_submit) ~= "function" then
		fail("open requires submit callback")
	end
	if opts.on_cancel ~= nil and type(opts.on_cancel) ~= "function" then
		fail("on_cancel must be a function")
	end
	if not settings.enabled then
		vim.ui.input({ prompt = opts.prompt or "Gator: " }, function(value)
			if type(value) == "string" and vim.trim(value) ~= "" then
				opts.on_submit(value)
			elseif opts.on_cancel then
				opts.on_cancel()
			end
		end)
		return true
	end
	local existing = current()
	if existing then
		return false
	end
	local opened = panel_window.open("botright 6new")
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].bufhidden = "wipe"
	vim.bo[buffer].swapfile = false
	vim.bo[buffer].filetype = "gator-composer"
	vim.api.nvim_win_set_buf(opened.window, buffer)
	vim.wo[opened.window].number = false
	vim.wo[opened.window].relativenumber = false
	vim.wo[opened.window].signcolumn = "no"
	vim.wo[opened.window].cursorline = false
	local _, tabpage = current()
	local panel = {
		window = opened.window,
		buffer = buffer,
		previous = opened.previous,
		prompt = opts.prompt or "Gator: ",
		candidates = opts.candidates or { skills = {}, files = {} },
		on_submit = opts.on_submit,
		on_cancel = opts.on_cancel or function() end,
	}
	panels[tabpage] = panel
	vim.api.nvim_buf_set_lines(buffer, 0, -1, false, { "" })
	vim.bo[buffer].modifiable = true
	decorate(buffer)
	vim.api.nvim_win_set_cursor(panel.window, { 1, 0 })
	local function close(submit)
		local active, active_tabpage = current()
		if active ~= panel then
			return false
		end
		local value = table.concat(vim.api.nvim_buf_get_lines(buffer, 0, -1, false), "\n")
		panels[active_tabpage] = nil
		panel_window.close(panel.window, panel.previous)
		if submit and vim.trim(value) ~= "" then
			panel.on_submit(value)
		elseif not submit then
			panel.on_cancel()
		end
		return true
	end
	vim.api.nvim_create_autocmd("InsertCharPre", {
		buffer = buffer,
		callback = function()
			panel.inserted_character = true
		end,
	})
	vim.api.nvim_create_autocmd("TextChangedI", {
		buffer = buffer,
		callback = function()
			highlight_attachments(buffer)
			if panel.inserted_character then
				panel.inserted_character = false
				completion(panel)
			end
		end,
	})
	for _, lhs in ipairs({ "<BS>", "<C-h>" }) do
		vim.keymap.set("i", lhs, function()
			if vim.fn.pumvisible() == 1 then
				vim.fn.complete_stop()
			end
			return vim.keycode("<BS>")
		end, { buffer = buffer, expr = true, silent = true, desc = "Gator composer delete" })
	end
	vim.keymap.set({ "n", "i" }, "<C-s>", function()
		close(true)
	end, { buffer = buffer, silent = true, desc = "Gator composer submit" })
	vim.keymap.set({ "n", "i" }, "<Esc>", function()
		close(false)
	end, { buffer = buffer, silent = true, desc = "Gator composer cancel" })
	vim.keymap.set("n", "q", function()
		close(false)
	end, { buffer = buffer, silent = true, desc = "Gator composer cancel" })
	vim.cmd("startinsert")
	return panel.window
end

return M
