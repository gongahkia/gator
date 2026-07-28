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
	content:find("You\nInspect this", 1, true)
		and content:find("Gator agent\nThe prior result", 1, true)
		and not content:find("## user", 1, true),
	"reopened structured chats must display persisted transcripts without raw Markdown headings"
)
conversation.update({ text = "I", state = "running", phase = "thinking", turn_started_at = os.time() })
conversation.update({ text = "'ll inspect", append = true, state = "running", phase = "responding" })
content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(
	content:find("I'll inspect", 1, true) and not content:find("I\n'll inspect", 1, true),
	"streaming assistant fragments must remain one rendered line"
)
assert(
	content:find("responding · 0:00", 1, true)
		and content:find("c cancel", 1, true)
		and not content:find("i prompt", 1, true),
	"active chats must show a safe phase, elapsed time, and cancel action instead of an idle prompt"
)
conversation.update({ state = "waiting_input" })
content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(
	content:find("i prompt", 1, true) and not content:find("c cancel", 1, true),
	"settled chats must restore the prompt action and remove cancellation"
)
conversation.update({ text = "See [health.lua](/private/path/health.lua) and `check()`.", state = "waiting_input" })
content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(
	content:find("See health.lua and check().", 1, true)
		and not content:find("/private/path/health.lua", 1, true)
		and not content:find("`check()`", 1, true),
	"chat display must render Markdown links and inline code without exposing raw markup targets"
)
assert(conversation.close(), "rehydrated conversations must remain ephemeral")
