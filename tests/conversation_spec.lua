local accessibility = require("gator.ui.accessibility")
local conversation = require("gator.ui.conversation")

accessibility.configure({ keymaps = {}, screen_reader = false, icons = "none" })
local window = conversation.open({
	provider = "codex",
	session_id = "thread-existing",
	run_id = "run-existing",
	state = "waiting_input",
	history = { "## user\nInspect this", "## assistant\nThe prior result" },
	on_input = function() end,
	on_cancel = function() end,
})
local content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(
	content:find("## user\nInspect this", 1, true) and content:find("## assistant\nThe prior result", 1, true),
	"reopened structured chats must rehydrate their persisted Gator transcript"
)
conversation.update({ text = "I", state = "running" })
conversation.update({ text = "'ll inspect", append = true, state = "running" })
content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(
	content:find("I'll inspect", 1, true) and not content:find("I\n'll inspect", 1, true),
	"streaming assistant fragments must remain one rendered line"
)
assert(conversation.close(), "rehydrated conversations must remain ephemeral")
