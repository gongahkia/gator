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
	fork = function() end,
	attach_context = function() end,
	review = function() end,
	launch_parallel = function() end,
	runbooks = function()
		return { { id = "parallel-review" } }
	end,
	runbook_status = function()
		return {
			id = "parallel-review",
			active = 1,
			reported_tokens = 20,
			max_tokens = 100,
			usage_state = "partial",
			steps = {
				{ id = "research", role = "researcher", state = "completed", ready = false, depends_on = {} },
				{ id = "write", role = "writer", state = "pending", ready = true, depends_on = { "research" } },
			},
		}
	end,
	start_ready_runbook_step = function() end,
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
assert(
	content:find("Gator runbooks", 1, true)
		and content:find("parallel-review · active 1 · reported 20/100 · usage partial", 1, true)
		and content:find("> write · writer · pending · depends research", 1, true),
	"run graph must show runbook dependencies and ready steps without a scheduler"
)
assert(graph.close(), "run graph must close its ephemeral panel")
