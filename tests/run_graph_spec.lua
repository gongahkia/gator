local accessibility = require("gator.ui.accessibility")
local graph = require("gator.ui.run_graph")

accessibility.configure({ keymaps = {}, screen_reader = false, icons = "none" })
local root = vim.fn.getcwd()
local window = graph.open({
	runs = function()
		return {
			{
				id = "run-graph",
				provider = "codex",
				role = "writer",
				state = "running",
				transport = "chat",
				workspace = { kind = "worktree", root = root .. "/worktree" },
				bundle_id = "bundle-graph",
				transcript = "available",
				usage = { state = "reported", input_tokens = 12, output_tokens = 8, total_tokens = 20 },
				budget = { limit_tokens = 100, action = "warn", state = "tracking" },
			},
		}
	end,
	focus = function() end,
	handoff = function() end,
	launch_parallel = function() end,
	stop = function() end,
	resume = function() end,
})
local content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(
	content:find("Workspace: worktree", 1, true)
		and content:find("Context: bundle-graph · transcript available", 1, true)
		and content:find("Usage: reported · in 12 · out 8 · total 20", 1, true)
		and content:find("Budget: tracking · 20/100", 1, true),
	"run graph must render workspace, context bundle, exact usage, and budget state"
)
assert(graph.close(), "run graph must close its ephemeral panel")
