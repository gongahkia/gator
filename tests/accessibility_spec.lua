local accessibility = require("gator.ui").accessibility
local config = require("gator.config")
local buffer = vim.api.nvim_create_buf(false, true)
accessibility.bind(buffer, { confirm = "<CR>" }, {
	confirm = function() end,
})
vim.api.nvim_buf_call(buffer, function()
	local mapping = vim.fn.maparg("<CR>", "n", false, true)
	assert(
		mapping.buffer == 1 and mapping.desc == "Gator confirm",
		"accessibility keymaps must be buffer-local and keyboard-addressable"
	)
end)
accessibility.text(buffer, { "plain text", "for screen readers" })
assert(
	vim.bo[buffer].filetype == "gator-text" and not vim.bo[buffer].modifiable,
	"accessibility text buffers must remain plain and read-only"
)
assert(
	config.resolve({ ui = { keymaps = { confirm = "<CR>" }, screen_reader = true } }).ui.keymaps.confirm == "<CR>",
	"user keymaps must be retained"
)
local ok = pcall(accessibility.bind, buffer, { unsupported = "x" }, {})
assert(not ok, "unsupported accessibility actions must fail explicitly")
