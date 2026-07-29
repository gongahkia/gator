local accessibility = require("gator.ui.accessibility")
local workspace = require("gator.ui.workspace")

accessibility.configure({ keymaps = {}, screen_reader = false, icons = "none" })
workspace.configure({
	position = "right",
	width = 30,
	height = 10,
	rotation = { "right", "bottom", "left" },
	panels = { context = true, activity = true, approvals = true },
	approval_scope = "all",
	images = { enabled = false },
})

local detached, cancelled, opened_runs = 0, 0, 0
local window = workspace.open({
	run_id = "run-workspace",
	provider = "codex",
	state = "waiting_input",
	history = { "## user\nInspect this", "## assistant\nReady" },
	context = {
		files = { "lua/gator/workflow.lua" },
		selections = { "L1-L2" },
		diagnostics = { "L3 warning" },
		images = {},
	},
	on_input = function() end,
	on_cancel = function()
		cancelled = cancelled + 1
	end,
	on_detach = function()
		detached = detached + 1
	end,
	on_runs = function()
		opened_runs = opened_runs + 1
	end,
})
local content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(
	content:find("## Context", 1, true)
		and content:find("lua/gator/workflow.lua", 1, true)
		and content:find("## Activity", 1, true)
		and content:find("You", 1, true)
		and content:find("## Approvals · 0 pending", 1, true),
	"run workspaces must render persistent context, activity, and approval panels"
)

local decisions = {}
assert(
	workspace.request_approval("run-workspace", {
		action = "modify files",
		details = { path = "lua/gator/workflow.lua" },
		on_decide = function(value)
			table.insert(decisions, value)
		end,
	}),
	"workspace must queue governed approvals"
)
vim.api.nvim_feedkeys("1", "x", false)
assert(decisions[1] == "approved", "numeric workspace shortcuts must resolve queued approvals")
assert(workspace.rotate("run-workspace"), "workspace layouts must rotate without recreating the run")
assert(workspace.detach("run-workspace") and detached == 1, "detaching must keep the run-owned workspace reusable")
assert(
	workspace.open({
		run_id = "run-workspace",
		provider = "codex",
		state = "waiting_input",
		on_input = function() end,
		on_cancel = function()
			cancelled = cancelled + 1
		end,
		on_detach = function()
			detached = detached + 1
		end,
		on_runs = function()
			opened_runs = opened_runs + 1
		end,
	}),
	"detached runs must reattach to a workspace"
)
assert(workspace.close(), "reattached workspace must close cleanly")
assert(detached == 2 and cancelled == 0 and opened_runs == 0, "detach must not cancel the underlying run")
