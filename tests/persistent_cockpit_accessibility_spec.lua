local gator = require("gator").setup({
	ui = {
		keymaps = { next = "]", previous = "[", confirm = "c", cancel = "x", help = "h" },
		screen_reader = true,
	},
})
local context = require("gator.context.pack")
local run = require("gator.core.run")
local task = require("gator.core.task")
local ui = require("gator.ui")

local function content(window)
	return table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
end

local function press(lhs)
	vim.api.nvim_feedkeys(vim.api.nvim_replace_termcodes(lhs, true, false, true), "xt", false)
end

local function mapping(window, lhs, action)
	local value = vim.api.nvim_buf_call(vim.api.nvim_win_get_buf(window), function()
		return vim.fn.maparg(lhs, "n", false, true)
	end)
	assert(value.buffer == 1 and value.desc == "Gator " .. action, "cockpit must expose configured " .. action)
end

local state = gator._state
state.tasks = {
	task.new({
		id = "task-cockpit-accessibility",
		objective = "Exercise persistent cockpit accessibility",
		lifecycle = "awaiting_review",
		workspace = { kind = "project", root = vim.g.gator_test.root },
		sessions = { { provider = "codex", id = "native-cockpit-accessibility", owner = "provider" } },
		created_at = 1,
		updated_at = 1,
	}),
}
state.context = {
	selections = {
		{
			entry = context.entry({
				id = "selection-cockpit-accessibility",
				kind = "selection",
				ref = "buffer:1-2",
				provenance = { source = "buffer", ref = "buffer:1-2" },
				trust = "manual",
				token_estimate = { status = "unavailable", reason = "not counted" },
				transfer = { eligible = true },
			}),
		},
	},
}
state.adapters = { codex = { available = true } }
state.review = {
	run = run.new({
		id = "run-cockpit-accessibility",
		task_id = "task-cockpit-accessibility",
		provider = { name = "codex", session_id = "native-cockpit-accessibility" },
		process = { pid = 1, executable = "codex" },
		workspace = { kind = "project", root = vim.g.gator_test.root },
		state = "completed",
		timing = {},
		usage = {},
	}),
	changes = { { path = "README.md", before = "old\n", after = "new\n" } },
}

vim.cmd("enew")
local user_window = vim.api.nvim_get_current_win()
local user_buffer = vim.api.nvim_get_current_buf()
local window = ui.open(state)
local buffer = vim.api.nvim_win_get_buf(window)

assert(vim.bo[buffer].filetype == "gator-text" and not vim.bo[buffer].modifiable, "cockpit must remain readable")
for lhs, action in pairs({ ["]"] = "next", ["["] = "previous", c = "confirm", x = "cancel", h = "help" }) do
	mapping(window, lhs, action)
end
assert(
	content(window):find("task%-cockpit%-accessibility")
		and content(window):find("Sessions: 1 linked provider%-native session%(s%)")
		and content(window):find("captured provenance")
		and content(window):find("Review: ready"),
	"cockpit must retain task, session, context, and review state in plain text"
)

press("]")
assert(content(window):find("> Open linked sessions", 1, true), "next key must advance the cockpit action")
press("[")
assert(content(window):find("> Open task dashboard", 1, true), "previous key must restore the cockpit action")

press("c")
assert(
	vim.bo[vim.api.nvim_win_get_buf(vim.api.nvim_get_current_win())].filetype == "gator-text",
	"dashboard must stay readable"
)
assert(ui.dashboard.close(), "dashboard must close without discarding the cockpit")
ui.focus()
press("]")
press("c")
assert(
	vim.bo[vim.api.nvim_win_get_buf(vim.api.nvim_get_current_win())].filetype == "gator-text",
	"sessions must stay readable"
)
assert(ui.sidebar.close(), "session panel must close without discarding the cockpit")
ui.focus()
press("]")
press("c")
assert(
	vim.bo[vim.api.nvim_win_get_buf(vim.api.nvim_get_current_win())].filetype == "gator-text",
	"context must stay readable"
)
assert(ui.context_inspector.close(), "context panel must close without discarding the cockpit")
ui.focus()
press("]")
press("c")
assert(
	vim.bo[vim.api.nvim_win_get_buf(vim.api.nvim_get_current_win())].filetype == "gator-text",
	"review must stay readable"
)
assert(ui.diff_review.close(), "review panel must close without discarding the cockpit")

ui.focus()
state:update({ workspace = { status = "degraded", detail = "accessibility provider status" } })
assert(
	vim.api.nvim_win_is_valid(window)
		and content(window):find("State: degraded · accessibility provider status", 1, true)
		and content(window):find("Degraded: some provider features are unavailable", 1, true),
	"cockpit must react to state changes without losing its readable panel"
)
state:update({ review = {} })
press("c")
assert(
	vim.api.nvim_win_is_valid(window)
		and content(window):find("State: failed · review evidence is unavailable", 1, true),
	"unavailable review evidence must fail in the persistent cockpit"
)

press("x")
assert(
	vim.api.nvim_get_current_win() == user_window and vim.api.nvim_win_get_buf(user_window) == user_buffer,
	"cancelling the cockpit must restore the prior user focus"
)
window = ui.restore(state)
assert(
	vim.bo[vim.api.nvim_win_get_buf(window)].filetype == "gator-text"
		and content(window):find("State: failed · review evidence is unavailable", 1, true),
	"restoring the cockpit must preserve its readable failure state"
)
assert(ui.close(), "restored cockpit must close cleanly")
gator.setup()
