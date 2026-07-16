local dashboard = require("gator.ui").workspace_dashboard
local window = dashboard.open({
	workspaces = {
		{
			id = "worktree-one",
			kind = "worktree",
			root = "/tmp/gator-worktree",
			tasks = { "task-one" },
			dirty_files = { "lua/gator/init.lua" },
			activity = { { provider = "codex", session_id = "native-one", state = "running" } },
		},
	},
})

assert(vim.api.nvim_win_is_valid(window), "workspace dashboard opening must create a window")
local content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(content:find("task%-one"), "workspace dashboard must render linked tasks")
assert(content:find("lua/gator/init.lua", 1, true), "workspace dashboard must render dirty files")
assert(content:find("native%-one"), "workspace dashboard must render agent activity")
assert(dashboard.select(1).kind == "worktree", "workspace dashboard selection must preserve workspace kind")
assert(dashboard.close(), "workspace dashboard close must report success")

local ok = pcall(dashboard.open, { workspaces = { { id = "invalid", kind = "remote", root = "/tmp", activity = {} } } })
assert(not ok, "unsupported workspace kinds must fail explicitly")
