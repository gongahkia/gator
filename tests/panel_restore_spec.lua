local picker = require("gator.ui").picker
local dashboard = require("gator.ui").dashboard
local sidebar = require("gator.ui").sidebar

vim.cmd("enew")
local user_window = vim.api.nvim_get_current_win()
local user_buffer = vim.api.nvim_get_current_buf()
local picker_window = picker.open({
	title = "Restore",
	items = { { id = "one", label = "One" } },
	on_select = function() end,
})
local dashboard_window = dashboard.open({
	tasks = {
		{
			id = "task-restore",
			objective = "restore focus",
			lifecycle = "running",
			provider = "codex",
			workspace = "project",
			review_state = "pending",
		},
	},
	on_open = function() end,
})
assert(
	dashboard.close() and vim.api.nvim_get_current_win() == picker_window,
	"nested panel close must restore the parent panel"
)
assert(
	picker.close()
		and vim.api.nvim_get_current_win() == user_window
		and vim.api.nvim_win_get_buf(user_window) == user_buffer,
	"top-level panel close must restore the prior user window and buffer"
)

local sidebar_window = sidebar.open({
	sessions = { { task_id = "task-restore", provider = "codex", id = "native-restore", streaming = false } },
	on_input = function() end,
})
assert(
	sidebar.close() and vim.api.nvim_get_current_win() == user_window and not vim.api.nvim_win_is_valid(sidebar_window),
	"sidebar close must restore the initiating user window"
)
