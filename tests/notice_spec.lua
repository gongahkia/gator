local notice = require("gator.ui.notice")

local original_list_uis = vim.api.nvim_list_uis
local original_notify = vim.notify
local notified = false
vim.api.nvim_list_uis = function()
	return { {} }
end
vim.notify = function()
	notified = true
end
local handle = notice.show("first line\nsecond line", vim.log.levels.ERROR, { timeout_ms = 0 })
vim.notify = original_notify
vim.api.nvim_list_uis = original_list_uis
local buffer = vim.tbl_filter(function(id)
	return vim.bo[id].filetype == "gator-notice"
end, vim.api.nvim_list_bufs())[1]
assert(not notified, "notices must not call vim.notify")
assert(
	buffer and #vim.api.nvim_buf_get_lines(buffer, 0, -1, false) == 1,
	"notices must constrain multi-line errors to one non-blocking floating line"
)
local window = vim.fn.win_findbuf(buffer)[1]
local config = vim.api.nvim_win_get_config(window)
local line = vim.api.nvim_buf_get_lines(buffer, 0, -1, false)[1]
assert(
	line == "first line second line"
		and config.anchor == "NE"
		and (config.row == 0 or config.row[false] == 0)
		and config.title[1][1] == "Gator",
	"notices must use a titled top-right float without obscuring command entry"
)
assert(handle.close(), "notices must close cleanly")
