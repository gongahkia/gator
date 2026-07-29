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
local backspace = vim.fn.maparg("<BS>", "i", false, true)
assert(
	type(backspace.callback) == "function" and backspace.callback() == vim.keycode("<BS>"),
	"composer backspace must preserve normal deletion after closing completion"
)
vim.api.nvim_buf_set_lines(buffer, 0, -1, false, { "@README.m" })
local submit = vim.fn.maparg("<C-s>", "i", false, true)
assert(type(submit.callback) == "function", "composer must expose an insert-mode submit action")
submit.callback()
assert(
	submitted == "@README.m",
	"composer backspace must remove attachment text before submission"
)
