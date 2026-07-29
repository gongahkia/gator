local accessibility = require("gator.ui.accessibility")
local panel_window = require("gator.ui.window")

local M = {}
local panels = {}
local settings = { enabled = true }

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
	local opened = panel_window.open("botright 9new")
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].bufhidden = "wipe"
	vim.bo[buffer].filetype = "gator-composer"
	vim.api.nvim_win_set_buf(opened.window, buffer)
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
	vim.api.nvim_buf_set_lines(buffer, 0, -1, false, { "# Gator composer", "# #directive · @tracked-file · <C-s> send · q cancel", "" })
	vim.bo[buffer].modifiable = true
	vim.api.nvim_win_set_cursor(panel.window, { 3, 0 })
	local function close(submit)
		local active, active_tabpage = current()
		if active ~= panel then
			return false
		end
		local value = table.concat(vim.api.nvim_buf_get_lines(buffer, 2, -1, false), "\n")
		panels[active_tabpage] = nil
		panel_window.close(panel.window, panel.previous)
		if submit and vim.trim(value) ~= "" then
			panel.on_submit(value)
		elseif not submit then
			panel.on_cancel()
		end
		return true
	end
	vim.api.nvim_create_autocmd("TextChangedI", {
		buffer = buffer,
		callback = function()
			completion(panel)
		end,
	})
	vim.keymap.set({ "n", "i" }, "<C-s>", function()
		close(true)
	end, { buffer = buffer, silent = true, desc = "Gator composer submit" })
	vim.keymap.set("n", "q", function()
		close(false)
	end, { buffer = buffer, silent = true, desc = "Gator composer cancel" })
	vim.cmd("startinsert")
	return panel.window
end

return M
