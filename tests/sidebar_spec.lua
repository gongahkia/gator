local sidebar = require("gator.ui").sidebar
local routed = {}
local user_window = vim.api.nvim_get_current_win()
local window = sidebar.open({
	sessions = {
		{ task_id = "task-sidebar", provider = "codex", id = "native-one", streaming = true },
		{ task_id = "task-sidebar", provider = "claude", id = "native-two", streaming = false },
	},
	on_input = function(session, text)
		routed = { session = session, text = text }
	end,
})

assert(vim.api.nvim_win_is_valid(window), "sidebar opening must create a window")
assert(sidebar.select(2).provider == "claude", "sidebar selection must preserve provider identity")
sidebar.route_input("review this diff")
assert(routed.session.id == "native-two", "sidebar input must route to the selected native session")
assert(routed.text == "review this diff", "sidebar input must preserve user text")
local first_tab = vim.api.nvim_get_current_tabpage()
vim.cmd("tabnew")
local second_tab = vim.api.nvim_get_current_tabpage()
local second_window = sidebar.open({
	sessions = { { task_id = "task-sidebar", provider = "gemini", id = "native-three", streaming = false } },
	on_input = function() end,
})
assert(sidebar.select(1).provider == "gemini", "sidebar selection must remain local to the current tab")
vim.api.nvim_set_current_tabpage(first_tab)
assert(sidebar.select(1).provider == "codex", "sidebar sessions must remain isolated by tabpage")
assert(sidebar.close(), "sidebar close must report success")
assert(vim.api.nvim_win_is_valid(user_window), "sidebar close must preserve user windows")
vim.api.nvim_set_current_tabpage(second_tab)
assert(vim.api.nvim_win_is_valid(second_window), "closing one tab sidebar must preserve other tab sidebars")
assert(sidebar.close(), "every tab-local sidebar must close independently")

local ok = pcall(sidebar.route_input, "orphaned")
assert(not ok, "sidebar input without a selected session must fail explicitly")
