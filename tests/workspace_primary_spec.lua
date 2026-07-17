local gator = require("gator").setup()
local context = require("gator.context.pack")
local run = require("gator.core.run")
local task = require("gator.core.task")
local ui = require("gator.ui")

local state = gator._state
state.tasks = {
	task.new({
		id = "task-workspace",
		objective = "Exercise the primary workspace",
		lifecycle = "awaiting_review",
		workspace = { kind = "project", root = vim.g.gator_test.root },
		sessions = { { provider = "codex", id = "native-workspace", owner = "provider" } },
		created_at = 1,
		updated_at = 1,
	}),
}
state.context.selections = {
	{
		entry = context.entry({
			id = "selection-workspace",
			kind = "selection",
			ref = "buffer:1-2",
			provenance = { source = "buffer", ref = "buffer:1-2" },
			trust = "manual",
			token_estimate = { status = "unavailable", reason = "not counted" },
			transfer = { eligible = true },
		}),
	},
}
state.adapters = { codex = { available = false, reason = "CLI not authenticated" } }
state.review = {
	run = run.new({
		id = "run-workspace",
		task_id = "task-workspace",
		provider = { name = "codex", session_id = "native-workspace" },
		process = { pid = 1, executable = "codex" },
		workspace = { kind = "project", root = vim.g.gator_test.root },
		state = "completed",
		timing = {},
		usage = {},
	}),
	changes = { { path = "README.md", before = "old\n", after = "new\n" } },
}

local window = ui.open(state)
local function content()
	return table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
end
assert(
	content():find("task%-workspace")
		and content():find("provider%-native session")
		and content():find("captured provenance")
		and content():find("Review: ready")
		and content():find("Provider codex: unavailable · CLI not authenticated", 1, true),
	"primary workspace must present task, session, context, review, and provider status"
)
assert(
	ui.set_status(state, "loading", "refreshing providers").status == "loading",
	"workspace must expose loading state"
)
assert(content():find("State: loading · refreshing providers", 1, true), "loading state must render explicitly")
ui.set_status(state, "failed", "provider probe failed")
assert(content():find("State: failed · provider probe failed", 1, true), "failure state must render explicitly")
ui.set_status(state, "recovering", "retrying provider probe")
assert(content():find("State: recovering · retrying provider probe", 1, true), "recovery state must render explicitly")
ui.set_status(state, "ready", "workspace synchronized")

local function press(lhs)
	vim.api.nvim_feedkeys(vim.api.nvim_replace_termcodes(lhs, true, false, true), "xt", false)
end

press("<CR>")
assert(vim.api.nvim_win_is_valid(vim.api.nvim_get_current_win()), "keyboard confirmation must open the task action")
assert(ui.dashboard.close(), "task dashboard action must be recoverable from the primary workspace")
ui.focus()
press("j")
press("<CR>")
assert(ui.sidebar.close(), "session action must be keyboard reachable")
ui.focus()
press("j")
press("<CR>")
assert(ui.context_inspector.close(), "context action must be keyboard reachable")
ui.focus()
press("j")
press("<CR>")
assert(ui.diff_review.close(), "review action must be keyboard reachable")
assert(ui.close(), "primary workspace must close cleanly after action recovery")
