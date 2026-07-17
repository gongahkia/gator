local M = {}
local renderers = {}
local namespace = vim.api.nvim_create_namespace("gator-markdown")
local accessibility = require("gator.ui.accessibility")

local function fail(message)
	error("Gator markdown: " .. message, 3)
end

local function current()
	local tabpage = vim.api.nvim_get_current_tabpage()
	local renderer = renderers[tabpage]
	if renderer and vim.api.nvim_win_is_valid(renderer.window) then
		return renderer, tabpage
	end
	renderers[tabpage] = nil
	return nil, tabpage
end

local function blocks(text)
	local result = {}
	local open
	for line_number, line in ipairs(vim.split(text, "\n", { plain = true, trimempty = false })) do
		if not open then
			local language = line:match("^%s*```%s*([^%s`]*)%s*$")
			if language then
				open = {
					language = language == "" and "plain" or language,
					fence_start = line_number,
					start_line = line_number + 1,
				}
			end
		elseif line:match("^%s*```%s*$") then
			open.fence_end = line_number
			open.end_line = line_number - 1
			open.complete = true
			table.insert(result, open)
			open = nil
		end
	end
	if open then
		open.end_line = #vim.split(text, "\n", { plain = true, trimempty = false })
		open.complete = false
		table.insert(result, open)
	end
	return result
end

local function tail(buffer)
	local lines = vim.api.nvim_buf_get_lines(buffer, -2, -1, false)
	return vim.api.nvim_buf_line_count(buffer), #(lines[1] or "")
end

local function follows_tail(renderer)
	if vim.api.nvim_get_current_buf() ~= renderer.buffer then
		return false
	end
	local row, column = unpack(vim.api.nvim_win_get_cursor(0))
	local tail_row, tail_column = tail(renderer.buffer)
	return row == tail_row and column == tail_column
end

local function render(renderer)
	local lines = vim.split(renderer.text, "\n", { plain = true, trimempty = false })
	accessibility.render(renderer.buffer, lines, "gator-markdown")
	renderer.blocks = blocks(renderer.text)
	vim.api.nvim_buf_clear_namespace(renderer.buffer, namespace, 0, -1)
	for _, block in ipairs(renderer.blocks) do
		vim.api.nvim_buf_set_extmark(renderer.buffer, namespace, block.fence_start - 1, 0, {
			virt_text = { { "  " .. block.language, "Comment" } },
			virt_text_pos = "eol",
		})
	end
end

local function bind(renderer)
	accessibility.panel(renderer.buffer, { cancel = "q", help = "?" }, {
		cancel = M.close,
		help = function()
			vim.notify("Gator markdown: use native movement keys; q closes the panel", vim.log.levels.INFO)
		end,
	})
end

function M.open()
	local renderer, tabpage = current()
	if renderer then
		vim.api.nvim_set_current_win(renderer.window)
		return renderer.window
	end
	vim.cmd("botright 16new")
	local window = vim.api.nvim_get_current_win()
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype = "gator-markdown"
	vim.bo[buffer].bufhidden = "wipe"
	vim.api.nvim_win_set_buf(window, buffer)
	renderer = { window = window, buffer = buffer, text = "", blocks = {} }
	renderers[tabpage] = renderer
	render(renderer)
	bind(renderer)
	return window
end

function M.append(text)
	local renderer = current()
	if not renderer then
		fail("no markdown renderer is open in this tab")
	end
	if type(text) ~= "string" or text == "" then
		fail("stream chunk must be a non-empty string")
	end
	local follow = follows_tail(renderer)
	renderer.text = renderer.text .. text
	render(renderer)
	if follow and vim.api.nvim_get_current_buf() == renderer.buffer then
		vim.api.nvim_win_set_cursor(0, { tail(renderer.buffer) })
	end
	return vim.deepcopy(renderer.blocks)
end

function M.code_blocks()
	local renderer = current()
	if not renderer then
		fail("no markdown renderer is open in this tab")
	end
	return vim.deepcopy(renderer.blocks)
end

function M.close()
	local renderer, tabpage = current()
	if not renderer then
		return false
	end
	vim.api.nvim_win_close(renderer.window, true)
	renderers[tabpage] = nil
	return true
end

return M
