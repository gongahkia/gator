local dashboard = require("gator.ui").dashboard
local tasks = {
	{
		id = "task-b",
		objective = "B",
		lifecycle = "running",
		provider = "codex",
		workspace = "worktree",
		review_state = "pending",
	},
	{
		id = "task-a",
		objective = "A",
		lifecycle = "planned",
		provider = "claude",
		workspace = "project",
		review_state = "none",
	},
}
local filtered = dashboard.filter(tasks, { provider = "codex", workspace = "worktree" })
assert(#filtered == 1 and filtered[1].id == "task-b", "dashboard filters must match task metadata")

local opened
local window = dashboard.open({
	tasks = tasks,
	filters = { lifecycle = "running" },
	on_open = function(task)
		opened = task
	end,
})
assert(vim.api.nvim_win_is_valid(window), "dashboard open must create a window")
assert(dashboard.select(1).id == "task-b", "dashboard selection must preserve task identity")
dashboard.open_selected()
assert(opened.id == "task-b" and opened.provider == "codex", "dashboard open must route the selected task")
assert(dashboard.close(), "dashboard close must report success")

local ok = pcall(dashboard.filter, tasks, { unsupported = "x" })
assert(not ok, "unsupported dashboard filters must fail explicitly")
