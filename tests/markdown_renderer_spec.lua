local markdown = require("gator.ui").markdown
local window = markdown.open()

assert(vim.api.nvim_win_is_valid(window), "markdown renderer opening must create a window")
local partial = markdown.append("Intro\n```lua\nlocal value")
assert(
	#partial == 1 and partial[1].language == "lua" and not partial[1].complete,
	"partial fenced code must remain syntax-aware"
)

vim.api.nvim_win_set_cursor(window, { 1, 0 })
local complete = markdown.append(" = 1\n```\n")
assert(vim.api.nvim_win_get_cursor(window)[1] == 1, "streaming must not move a reader cursor away from its position")
assert(
	#complete == 1 and complete[1].language == "lua" and complete[1].complete,
	"completed code blocks must retain their language"
)

local line_count = vim.api.nvim_buf_line_count(vim.api.nvim_win_get_buf(window))
vim.api.nvim_win_set_cursor(window, { line_count, 0 })
markdown.append("tail")
assert(
	vim.api.nvim_win_get_cursor(window)[1] == vim.api.nvim_buf_line_count(vim.api.nvim_win_get_buf(window)),
	"tail-following must remain stable"
)
assert(markdown.close(), "markdown renderer close must report success")
