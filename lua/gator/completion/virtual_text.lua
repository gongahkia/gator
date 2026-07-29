local M = {}

local namespace = vim.api.nvim_create_namespace("gator_completion")
local state = { status = "idle", current = nil, settings = nil, request = nil, installed = {} }

local function clear_marks(buffer)
	if buffer and vim.api.nvim_buf_is_valid(buffer) then
		vim.api.nvim_buf_clear_namespace(buffer, namespace, 0, -1)
	end
end

local function less_or_equal(left, right)
	return left.line < right.line or (left.line == right.line and left.byte_column <= right.byte_column)
end

local function position(value)
	if type(value) ~= "table" or type(value.line) ~= "number" or value.line < 0 or value.line % 1 ~= 0 then
		return nil
	end
	local column = value.byte_column
	if column == nil then
		column = value.character
	end
	if type(column) ~= "number" or column < 0 or column % 1 ~= 0 then
		return nil
	end
	return { line = value.line, byte_column = column }
end

local function candidate(item, document)
	if type(item) ~= "table" then
		return nil
	end
	local text = item.text or item.insert_text
	if type(text) ~= "string" or text == "" or text:find("%z") then
		return nil
	end
	local start, finish
	if item.range then
		start = position(item.range.start)
		finish = position(item.range["end"])
		if not start or not finish or not less_or_equal(start, finish) then
			return nil
		end
	else
		start = vim.deepcopy(document.cursor)
		finish = vim.deepcopy(document.cursor)
	end
	local cursor = document.cursor
	if
		start.line < document.window.first_line
		or finish.line > document.window.last_line
		or not less_or_equal(start, cursor)
		or not less_or_equal(cursor, finish)
	then
		return nil
	end
	return { id = type(item.id) == "string" and item.id or nil, text = text, range = { start = start, ["end"] = finish } }
end

local function render()
	local current = state.current
	if not current or not current.items[current.index] then
		return
	end
	local item = current.items[current.index]
	local first, rest = item.text:match("^([^\n]*)(.*)$")
	local options = {
		virt_text = { { first, "GatorCompletionSuggestion" } },
		virt_text_pos = "overlay",
		priority = state.settings.ui.virtual_text.priority,
	}
	if rest and rest ~= "" then
		local lines = vim.split(rest:sub(2), "\n", { plain = true, trimempty = false })
		options.virt_lines = vim.tbl_map(function(line)
			return { { line, "GatorCompletionSuggestion" } }
		end, lines)
	end
	vim.api.nvim_buf_set_extmark(current.buffer, namespace, item.range.start.line, item.range.start.byte_column, options)
end

local function reset(cancel)
	local current = state.current
	if current then
		clear_marks(current.buffer)
		if cancel and current.ticket then
			state.cancel(current.ticket)
		end
	end
	state.current = nil
	state.status = "idle"
end

local function apply(text)
	local current = state.current
	local item = current and current.items[current.index] or nil
	if not item or not vim.api.nvim_buf_is_valid(current.buffer) then
		return ""
	end
	if vim.api.nvim_buf_get_changedtick(current.buffer) ~= current.document.version then
		reset(true)
		return ""
	end
	local cursor = vim.api.nvim_win_get_cursor(0)
	if vim.api.nvim_get_current_buf() ~= current.buffer or cursor[1] - 1 ~= current.document.cursor.line or cursor[2] ~= current.document.cursor.byte_column then
		reset(true)
		return ""
	end
	vim.api.nvim_buf_set_text(
		current.buffer,
		item.range.start.line,
		item.range.start.byte_column,
		item.range["end"].line,
		item.range["end"].byte_column,
		vim.split(text, "\n", { plain = true, trimempty = false })
	)
	reset(false)
	return ""
end

local function mapping(lhs)
	local value = vim.fn.maparg(lhs, "i", false, true)
	return type(value) == "table" and next(value) and value or nil
end

local function remove_mapping(name)
	local installed = state.installed[name]
	if not installed then
		return
	end
	local current = mapping(installed)
	if current and current.rhs == "<Plug>(gator-completion-" .. name .. ")" then
		vim.keymap.del("i", installed)
	end
	state.installed[name] = nil
end

function M.setup(opts)
	state.settings, state.request, state.cancel = opts.settings, opts.request, opts.cancel
	reset(true)
	for _, name in ipairs({ "accept", "accept_word", "accept_line", "clear", "next", "prev" }) do
		vim.keymap.set("i", "<Plug>(gator-completion-" .. name .. ")", function()
			return M[name]()
		end, { desc = "Gator completion " .. name, expr = true, silent = true })
		remove_mapping(name)
		local lhs = state.settings.ui.keymaps[name]
		if lhs and not mapping(lhs) then
			vim.keymap.set("i", lhs, "<Plug>(gator-completion-" .. name .. ")", {
				desc = "Gator completion " .. name,
				expr = true,
				silent = true,
			})
			state.installed[name] = lhs
		end
	end
	vim.api.nvim_set_hl(0, "GatorCompletionSuggestion", { link = "Comment", default = true })
	local group = vim.api.nvim_create_augroup("GatorCompletionVirtualText", { clear = true })
	if not state.settings.ui.virtual_text.enabled then
		return
	end
	vim.api.nvim_create_autocmd({ "InsertEnter", "TextChangedI", "CursorMovedI" }, {
		group = group,
		callback = function()
			M.debounced_complete()
		end,
	})
	vim.api.nvim_create_autocmd({ "InsertLeave", "BufLeave" }, { group = group, callback = M.clear })
	vim.api.nvim_create_autocmd("ColorScheme", { group = group, callback = function()
		vim.api.nvim_set_hl(0, "GatorCompletionSuggestion", { link = "Comment", default = true })
	end })
end

function M.complete()
	if not state.settings or not state.settings.enabled or not state.settings.ui.virtual_text.enabled then
		return false
	end
	reset(true)
	local buffer = vim.api.nvim_get_current_buf()
	local serial = vim.uv.now()
	state.status = "waiting"
	local document, ticket = state.request(buffer, function(items, error, payload)
		vim.schedule(function()
			if not state.current or state.current.serial ~= serial then
				return
			end
			if error or not items then
				state.status = "idle"
				state.current = nil
				return
			end
			local values = {}
			for _, item in ipairs(items) do
				local value = candidate(item, payload.document)
				if value then
					table.insert(values, value)
				end
			end
			if #values == 0 then
				state.status, state.current = "idle", nil
				return
			end
			state.current.items, state.current.index, state.status = values, 1, "completions"
			render()
		end)
	end)
	if not document then
		state.status = "idle"
		return false, ticket
	end
	state.current = { buffer = buffer, document = document, serial = serial, ticket = ticket }
	return true
end

function M.debounced_complete()
	if state.timer then
		state.timer:stop()
		state.timer:close()
	end
	state.timer = vim.uv.new_timer()
	state.timer:start(75, 0, vim.schedule_wrap(function()
		if state.timer then
			state.timer:stop()
			state.timer:close()
			state.timer = nil
		end
		if vim.fn.mode() == "i" then
			M.complete()
		end
	end))
end

function M.accept()
	local current = state.current
	return current and current.items and apply(current.items[current.index].text) or "\t"
end

function M.accept_word()
	local current = state.current
	local item = current and current.items and current.items[current.index] or nil
	if not item or item.range.start.line ~= item.range["end"].line or item.range.start.byte_column ~= item.range["end"].byte_column then
		return ""
	end
	return apply(item.text:match("^%W*%w*") or item.text)
end

function M.accept_line()
	local current = state.current
	local item = current and current.items and current.items[current.index] or nil
	if not item or item.range.start.line ~= item.range["end"].line or item.range.start.byte_column ~= item.range["end"].byte_column then
		return ""
	end
	return apply((item.text:match("^[^\n]*") or item.text))
end

function M.clear()
	reset(true)
	return ""
end

function M.next()
	if state.current and state.current.items then
		clear_marks(state.current.buffer)
		state.current.index = state.current.index % #state.current.items + 1
		render()
	end
	return ""
end

function M.prev()
	if state.current and state.current.items then
		clear_marks(state.current.buffer)
		state.current.index = (state.current.index - 2) % #state.current.items + 1
		render()
	end
	return ""
end

function M.status()
	local total = state.current and state.current.items and #state.current.items or 0
	return { state = state.status, current = state.current and state.current.index or 0, total = total }
end

function M.status_string()
	local value = M.status()
	if value.state == "waiting" then
		return "*"
	end
	if value.state == "completions" then
		return value.current .. "/" .. value.total
	end
	return "0"
end

function M.close()
	if state.timer then
		state.timer:stop()
		state.timer:close()
		state.timer = nil
	end
	reset(true)
end

return M
