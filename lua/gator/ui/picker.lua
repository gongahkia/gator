local M = {}
local pickers = {}
local accessibility = require("gator.ui.accessibility")

local function fail(message)
	error("Gator picker: " .. message, 3)
end

local function items(value)
	if type(value) ~= "table" or not vim.islist(value) then
		fail("items must be an array")
	end
	local result, ids = {}, {}
	for index, item in ipairs(value) do
		if type(item) ~= "table" then
			fail("item " .. index .. " must be a table")
		end
		for key in pairs(item) do
			if key ~= "id" and key ~= "label" then
				fail("item " .. index .. " contains unsupported field: " .. tostring(key))
			end
		end
		if type(item.id) ~= "string" or item.id == "" or type(item.label) ~= "string" or item.label == "" then
			fail("item " .. index .. " id and label must be non-empty strings")
		end
		if ids[item.id] then
			fail("item ids must be unique: " .. item.id)
		end
		ids[item.id] = true
		result[index] = vim.deepcopy(item)
	end
	return result
end

local function current()
	local tabpage = vim.api.nvim_get_current_tabpage()
	local picker = pickers[tabpage]
	if picker and vim.api.nvim_win_is_valid(picker.window) then
		return picker, tabpage
	end
	pickers[tabpage] = nil
	return nil, tabpage
end

local function render(picker)
	picker.visible = {}
	local query = picker.query:lower()
	for _, item in ipairs(picker.items) do
		if item.id:lower():find(query, 1, true) or item.label:lower():find(query, 1, true) then
			table.insert(picker.visible, item)
		end
	end
	picker.selected = math.min(picker.selected, math.max(#picker.visible, 1))
	local lines = { picker.title }
	if #picker.visible == 0 then
		table.insert(lines, "No matching items")
	end
	for index, item in ipairs(picker.visible) do
		table.insert(lines, (index == picker.selected and ">" or " ") .. " " .. item.label)
	end
	table.insert(lines, "j/k navigate · <CR> confirm · q cancel · ? help")
	accessibility.render(picker.buffer, lines, "gator-picker")
end

local function bind(picker)
	accessibility.panel(picker.buffer, { next = "j", previous = "k", confirm = "<CR>", cancel = "q", help = "?" }, {
		next = function()
			if #picker.visible > 0 then
				M.select(picker.selected % #picker.visible + 1)
			end
		end,
		previous = function()
			if #picker.visible > 0 then
				M.select((picker.selected - 2) % #picker.visible + 1)
			end
		end,
		confirm = function()
			if #picker.visible > 0 then
				M.confirm()
			end
		end,
		cancel = M.cancel,
		help = function()
			vim.notify("Gator picker: j/k navigate, <CR> confirm, q cancel", vim.log.levels.INFO)
		end,
	})
end

function M.open(opts)
	if
		type(opts) ~= "table"
		or type(opts.title) ~= "string"
		or opts.title == ""
		or type(opts.on_select) ~= "function"
	then
		fail("open requires title and on_select callback")
	end
	if opts.on_cancel ~= nil and type(opts.on_cancel) ~= "function" then
		fail("on_cancel must be a function")
	end
	local value = items(opts.items or {})
	local picker, tabpage = current()
	if picker then
		picker.title, picker.items, picker.query, picker.selected = opts.title, value, "", 1
		picker.on_select, picker.on_cancel = opts.on_select, opts.on_cancel
		render(picker)
		vim.api.nvim_set_current_win(picker.window)
		return picker.window
	end
	vim.cmd("botright 12new")
	local window = vim.api.nvim_get_current_win()
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype, vim.bo[buffer].bufhidden = "gator-picker", "wipe"
	vim.api.nvim_win_set_buf(window, buffer)
	picker = {
		window = window,
		buffer = buffer,
		title = opts.title,
		items = value,
		visible = {},
		query = "",
		selected = 1,
		on_select = opts.on_select,
		on_cancel = opts.on_cancel,
	}
	pickers[tabpage] = picker
	render(picker)
	bind(picker)
	return window
end

function M.filter(query)
	local picker = current()
	if not picker then
		fail("no picker is open in this tab")
	end
	if type(query) ~= "string" then
		fail("query must be a string")
	end
	picker.query, picker.selected = query, 1
	render(picker)
	return vim.deepcopy(picker.visible)
end

function M.select(index)
	local picker = current()
	if not picker then
		fail("no picker is open in this tab")
	end
	if type(index) ~= "number" or index % 1 ~= 0 or index < 1 or index > #picker.visible then
		fail("selection must identify a visible item")
	end
	picker.selected = index
	render(picker)
	return vim.deepcopy(picker.visible[index])
end

function M.confirm()
	local picker = current()
	if not picker or not picker.visible[picker.selected] then
		fail("no picker item is selected")
	end
	local item = vim.deepcopy(picker.visible[picker.selected])
	picker.on_select(item)
	M.close()
	return item
end

function M.cancel()
	local picker = current()
	if not picker then
		fail("no picker is open in this tab")
	end
	if picker.on_cancel then
		picker.on_cancel()
	end
	return M.close()
end

function M.close()
	local picker, tabpage = current()
	if not picker then
		return false
	end
	vim.api.nvim_win_close(picker.window, true)
	pickers[tabpage] = nil
	return true
end

return M
