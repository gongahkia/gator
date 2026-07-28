local accessibility = require("gator.ui.accessibility")
local conversation = require("gator.ui.conversation")

accessibility.configure({ keymaps = {}, screen_reader = false, icons = "none" })
local window = conversation.open({
	provider = "codex",
	session_id = "thread-existing",
	run_id = "run-existing",
	state = "waiting_input",
	trust = {
		surface = "structured",
		security_owner = "provider",
		provider = "codex",
		policy = { state = "applied", detail = "test", mode = "gator.config" },
		write = { state = "codex_enforced", detail = "test", mode = "workspace_write" },
		network = { state = "unknown", detail = "test" },
		mcp = { state = "unknown", detail = "test" },
		approval = { state = "on_request", detail = "test" },
	},
	workspace = { kind = "worktree", root = "/tmp/gator-worktree" },
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
assert(
	content:find(
		"Trust: policy gator.config · write workspace-write · approval on-request · worktree gator-worktree",
		1,
		true
	),
	"chat headers must expose the recorded policy, write mode, approval mode, and workspace"
)
assert(
	content:find("Ready for input (waiting_input)", 1, true)
		and content:find("Status: the provider turn is complete; i sends a follow-up", 1, true),
	"chat headers must explain run states in place"
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

conversation.configure({ layout = "float", height = 10, width = 40 })
window = conversation.open({
	provider = "codex",
	session_id = "thread-float",
	run_id = "run-float",
	state = "waiting_input",
	on_input = function() end,
	on_cancel = function() end,
})
assert(vim.api.nvim_win_get_config(window).relative == "editor", "configured chats must open in a floating window")
assert(conversation.resize(2), "floating chats must resize in place")
assert(conversation.toggle_fullscreen(), "chat controls must maximize the current conversation")
assert(conversation.cycle_layout(), "chat controls must cycle from fullscreen back to a split")
assert(conversation.close(), "resized chats must close cleanly")
conversation.configure({ layout = "split", height = 18, width = 0 })
