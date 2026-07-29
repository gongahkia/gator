local config = require("gator.config")
local context = require("gator.completion.context")

local path = vim.g.gator_test.root .. "/tests/fixtures/sample.txt"
vim.cmd("edit " .. vim.fn.fnameescape(path))
vim.api.nvim_buf_set_lines(0, 0, -1, false, { "token=private-value", "local value = 1", "return value" })
vim.api.nvim_win_set_cursor(0, { 2, 6 })
local settings =
	config.resolve({ completion = { context = { before_lines = 1, after_lines = 1, max_bytes = 128 } } }).completion
local payload = assert(context.document({ buffer = 0, settings = settings }))
assert(
	payload.document.text:find("[REDACTED]", 1, true)
		and payload.document.cursor.line == 1
		and payload.document.cursor.byte_column == 6,
	"completion context must redact bounded current-buffer content while retaining byte cursor metadata"
)
assert(payload.workspace.root == vim.g.gator_test.root, "Git root must be the default completion workspace")
assert(
	payload.context.mode == "bounded" and payload.context.redactions == 1,
	"completion context must report its delivery policy"
)
