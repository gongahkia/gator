local config = require("gator.config")
local virtual_text = require("gator.completion.virtual_text")

local settings = config.resolve().completion
local callback
virtual_text.setup({
	settings = settings,
	cancel = function()
		return true
	end,
	request = function(buffer, handler)
		callback = handler
		return {
			version = vim.api.nvim_buf_get_changedtick(buffer),
			window = { first_line = 0, last_line = 0 },
			cursor = { line = 0, byte_column = 6 },
		},
		{ root = "fixture", id = 1 }
	end,
})
vim.api.nvim_buf_set_lines(0, 0, -1, false, { "hello " })
vim.api.nvim_win_set_cursor(0, { 1, 6 })
assert(virtual_text.complete(), "manual completion must dispatch a request")
callback({ { text = "world" } }, nil, {
	document = {
		version = vim.api.nvim_buf_get_changedtick(0),
		window = { first_line = 0, last_line = 0 },
		cursor = { line = 0, byte_column = 6 },
	},
})
assert(vim.wait(100, function()
	return virtual_text.status().state == "completions"
end), "sidecar candidates must become ghost text")
virtual_text.accept()
assert(
	vim.api.nvim_get_current_line() == "hello world",
	"accepting a ghost suggestion must apply one buffer edit: " .. vim.inspect({ vim.api.nvim_get_current_line(), virtual_text.status() })
)

virtual_text.close()
