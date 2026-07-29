local redact = require("gator.policy.redact")

local M = {}
local current

local function fail(message)
	error("Gator notice: " .. message, 3)
end

local function compact(value)
	local text = redact.text(tostring(value)):gsub("[\r\n]+", " "):gsub("%s+", " ")
	return vim.trim(text)
end

local function truncate(value, width)
	if vim.fn.strdisplaywidth(value) <= width then
		return value
	end
	local result, index = "", 0
	while vim.fn.strdisplaywidth(result .. "…") < width do
		local character = vim.fn.strcharpart(value, index, 1)
		if character == "" then
			break
		end
		result, index = result .. character, index + 1
	end
	return result .. "…"
end

local function close(panel)
	if not panel or panel.closed then
		return false
	end
	panel.closed = true
	if panel.timer and not panel.timer:is_closing() then
		panel.timer:stop()
		panel.timer:close()
	end
	if vim.api.nvim_win_is_valid(panel.window) then
		vim.api.nvim_win_close(panel.window, true)
	end
	if current == panel then
		current = nil
	end
	return true
end

function M.show(message, level, opts)
	opts = opts or {}
	if type(opts) ~= "table" or (vim.islist(opts) and next(opts) ~= nil) then
		fail("options must be an object")
	end
	for key in pairs(opts) do
		if key ~= "title" and key ~= "timeout_ms" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	if type(message) ~= "string" and type(message) ~= "number" then
		fail("message must be text")
	end
	if level ~= nil and type(level) ~= "number" then
		fail("level must be a number")
	end
	if opts.title ~= nil and (type(opts.title) ~= "string" or opts.title == "") then
		fail("title must be non-empty text")
	end
	if
		opts.timeout_ms ~= nil
		and (type(opts.timeout_ms) ~= "number" or opts.timeout_ms < 0 or opts.timeout_ms % 1 ~= 0)
	then
		fail("timeout_ms must be a non-negative integer")
	end
	local text = compact(message)
	if text == "" then
		text = "Gator reported an empty notification"
	end
	if #vim.api.nvim_list_uis() == 0 then
		return { close = function() end }
	end
	close(current)
	local title = opts.title or "Gator"
	local maximum = math.max(math.min(vim.o.columns - 6, 100), 24)
	local line = truncate(text, maximum)
	local width = math.max(vim.fn.strdisplaywidth(line), vim.fn.strdisplaywidth(title), 1)
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].bufhidden, vim.bo[buffer].filetype = "wipe", "gator-notice"
	vim.bo[buffer].modifiable = true
	vim.api.nvim_buf_set_lines(buffer, 0, -1, false, { line })
	vim.bo[buffer].modifiable = false
	local window = vim.api.nvim_open_win(buffer, false, {
		relative = "editor",
		anchor = "NE",
		row = 0,
		col = vim.o.columns - 1,
		width = width,
		height = 1,
		style = "minimal",
		focusable = false,
		border = "rounded",
		title = title,
		title_pos = "left",
		zindex = 220,
		noautocmd = true,
	})
	local panel = { buffer = buffer, window = window, closed = false }
	current = panel
	local timeout = opts.timeout_ms or (level == vim.log.levels.ERROR and 7000 or 4000)
	if timeout > 0 then
		panel.timer = vim.uv.new_timer()
		panel.timer:start(
			timeout,
			0,
			vim.schedule_wrap(function()
				close(panel)
			end)
		)
	end
	return {
		close = function()
			return close(panel)
		end,
	}
end

function M.close()
	return close(current)
end

return M
