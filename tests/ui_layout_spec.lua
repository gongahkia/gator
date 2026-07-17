local gator = require("gator").setup()
local ui = require("gator.ui")

vim.cmd("enew")
local user_window = vim.api.nvim_get_current_win()
local user_buffer = vim.api.nvim_get_current_buf()
local panel = ui.open(gator._state)

assert(vim.api.nvim_win_is_valid(panel), "opening Gator must create a panel window")
assert(vim.api.nvim_get_current_win() == panel, "opening Gator must focus its panel")
assert(vim.api.nvim_win_get_buf(user_window) == user_buffer, "opening Gator must preserve the user buffer")
local lines = vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(panel), 0, -1, false)
assert(
	lines[1] == "Gator workspace"
		and vim.tbl_contains(lines, "Tasks: empty · create or import a task to begin")
		and vim.tbl_contains(lines, "Actions:"),
	"opening Gator must render the primary task/session/context/review workspace"
)
assert(vim.fn.maparg("q", "n", false, true).buffer == 1, "workspace close must be keyboard-accessible")
assert(ui.open(gator._state) == panel, "opening Gator twice must reuse its panel")
assert(ui.resize(6) == 6, "Gator panels must resize explicitly")
assert(ui.restore(gator._state) == panel, "restoring an open panel must focus it")
assert(ui.close(), "closing an open panel must report success")
assert(vim.api.nvim_get_current_win() == user_window, "closing Gator must restore user focus")
assert(vim.api.nvim_win_get_buf(user_window) == user_buffer, "closing Gator must preserve the user layout")
assert(not ui.close(), "closing an absent panel must be a no-op")

local ok = pcall(ui.resize, 1)
assert(not ok, "resizing an absent panel must fail explicitly")
