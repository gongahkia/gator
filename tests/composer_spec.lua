local composer = require("gator.ui.composer")

composer.configure({ enabled = true })
local submitted
local window = composer.open({
	candidates = { skills = {}, files = { { label = "@README.md", detail = "README.md" } } },
	on_submit = function(value)
		submitted = value
	end,
})
assert(vim.api.nvim_win_is_valid(window), "composer must open a dedicated input window")
local buffer = vim.api.nvim_win_get_buf(window)
assert(
	vim.api.nvim_buf_get_lines(buffer, 0, -1, false)[1] == "" and vim.fn.maparg("<BS>", "i", false, true).buffer == 1,
	"composer instructions must be decorative and attachment text must remain deletable"
)
vim.api.nvim_buf_set_lines(buffer, 0, -1, false, { "@README.md" })
vim.api.nvim_set_current_win(window)
vim.api.nvim_win_set_cursor(window, { 1, #"@README.md" })
vim.api.nvim_feedkeys(vim.keycode("A<BS><C-s>"), "xt", false)
assert(
	submitted == "@README.m",
	"composer backspace must remove attachment text before submission: " .. vim.inspect(submitted)
)
