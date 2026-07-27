local accessibility = require("gator.ui.accessibility")
local preflight = require("gator.ui.context_preflight")

accessibility.configure({ keymaps = {}, screen_reader = false, icons = "none" })
local confirmed, cancelled = false, false
local window = preflight.open({
	preflight = {
		provider = "codex",
		transport = "chat",
		bytes = 48,
		tokens = 12,
		redactions = 2,
		artifacts = { { kind = "selection", path = "src/main.lua", first_line = 2, last_line = 4, bytes = 32 } },
	},
	on_confirm = function()
		confirmed = true
	end,
	on_cancel = function()
		cancelled = true
	end,
})
local buffer = vim.api.nvim_win_get_buf(window)
local content = table.concat(vim.api.nvim_buf_get_lines(buffer, 0, -1, false), "\n")
assert(
	content:find("Target: codex · chat", 1, true)
		and content:find("src/main.lua · lines 2-4", 1, true)
		and content:find("redactions 2", 1, true),
	"preflight UI must show exact target and passive context metadata"
)
assert(preflight.close() and cancelled and not confirmed, "closing preflight must cancel without sending context")

window = preflight.open({
	preflight = {
		provider = "pi",
		transport = "chat",
		bytes = 1,
		tokens = 1,
		redactions = 0,
		artifacts = { { kind = "selection", bytes = 1 } },
	},
	on_confirm = function()
		confirmed = true
	end,
})
vim.api.nvim_set_current_win(window)
vim.api.nvim_feedkeys(vim.keycode("<CR>"), "xt", false)
assert(
	vim.wait(100, function()
		return confirmed
	end),
	"confirming preflight must invoke the explicit delivery callback"
)
assert(not preflight.close(), "confirmed preflight must close its ephemeral panel")
