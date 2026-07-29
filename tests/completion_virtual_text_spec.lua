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
		local cursor = vim.api.nvim_win_get_cursor(0)
		return {
			version = vim.api.nvim_buf_get_changedtick(buffer),
			window = { first_line = 0, last_line = 0 },
			cursor = { line = cursor[1] - 1, byte_column = cursor[2] },
		},
		{ root = "fixture", id = 1 }
	end,
})
vim.api.nvim_buf_set_lines(0, 0, -1, false, { "hello " })
vim.api.nvim_win_set_cursor(0, { 1, 6 })
assert(virtual_text.complete(), "manual completion must dispatch a request")
local cursor = vim.api.nvim_win_get_cursor(0)
local payload = {
	document = {
		version = vim.api.nvim_buf_get_changedtick(0),
		window = { first_line = 0, last_line = 0 },
		cursor = { line = cursor[1] - 1, byte_column = cursor[2] },
	},
}
callback({ { text = "world" } }, nil, payload)
assert(vim.wait(100, function()
	return virtual_text.status().state == "completions"
end), "sidecar candidates must become ghost text")
assert(vim.api.nvim_buf_get_changedtick(0) == payload.document.version, "rendering ghost text must not change buffer text")
virtual_text.accept()
assert(
	vim.api.nvim_get_current_line() == ("hello "):sub(1, cursor[2]) .. "world" .. ("hello "):sub(cursor[2] + 1),
	"accepting a ghost suggestion must apply one buffer edit"
)

virtual_text.close()
