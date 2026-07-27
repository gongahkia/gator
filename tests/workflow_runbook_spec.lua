local config = require("gator.config")
local state = require("gator.state")
local workflow = require("gator.workflow")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local root = helpers.tempdir("workflow-runbook")
assert(vim.system({ "git", "init", "-q" }, { cwd = root, text = true }):wait().code == 0, "fixture must initialize Git")
helpers.write(root .. "/baseline.lua", "return 'baseline'\n")
assert(
	vim.system({ "git", "add", "baseline.lua" }, { cwd = root, text = true }):wait().code == 0,
	"fixture must stage baseline"
)
assert(
	vim.system({ "git", "-c", "user.name=Gator", "-c", "user.email=gator@example.invalid", "commit", "-qm", "base" }, {
		cwd = root,
		text = true,
	})
		:wait().code == 0,
	"fixture must commit baseline"
)

local opened, review = {}, nil
local value = workflow.new({
	state = state.new(
		config.resolve({
			launch = { transport = "terminal" },
			providers = { pi = { user_confirmed = true } },
		}),
		{ supported = true }
	),
	root = root,
	readiness = function()
		return { { provider = "pi", available = true, readiness_state = "user_confirmed" } }
	end,
	bridge = {
		start = function(_, opts, callback)
			callback({
				session = { provider = opts.provider, id = "pi-session", owner = "provider" },
				command = { "pi", opts.prompt },
			})
		end,
	},
	terminal = {
		open = function(_, opts)
			table.insert(opened, opts)
			return { job_id = #opened }
		end,
		attach = function()
			return true
		end,
		close = function()
			return true
		end,
	},
	handoff_review = {
		open = function(opts)
			review = opts
			opts.on_confirm(opts.body, {})
			return true
		end,
	},
})

local runbook = value:create_runbook({
	id = "parallel-change",
	title = "Research, write, review, integrate",
	max_concurrent = 1,
	max_tokens = 0,
	steps = {
		{ id = "research", role = "researcher", objective = "Inspect the baseline", provider = "pi", depends_on = {} },
		{
			id = "write",
			role = "writer",
			objective = "Implement the change",
			provider = "pi",
			depends_on = { "research" },
		},
		{
			id = "review",
			role = "reviewer",
			objective = "Review the writer diff",
			provider = "pi",
			depends_on = { "write" },
		},
		{
			id = "integrate",
			role = "integrator",
			objective = "Converge reviewed work",
			provider = "pi",
			depends_on = { "write", "review" },
		},
	},
})
assert(runbook.id == "parallel-change", "runbooks must persist explicit role/provider/dependency declarations")
assert(
	not pcall(value.start_runbook_step, value, runbook.id, "write", false),
	"blocked dependencies must not launch automatically"
)

local research = value:start_runbook_step(runbook.id, "research", false)
assert(
	research.role == "researcher"
		and research.workspace.kind == "project"
		and opened[#opened].command[2]:find("do not edit files", 1, true),
	"researchers must use the project workspace with an explicit read-only instruction"
)
value:report_usage(research.id, { state = "reported", input_tokens = 1, output_tokens = 1, total_tokens = 2 })
value:update(research.id, { state = "completed" })
assert(
	value:runbook_status(runbook.id).steps[2].ready,
	"a completed dependency must expose its next manual runbook step as ready"
)

local writer = value:start_runbook_step(runbook.id, "write", false)
assert(
	writer.role == "writer"
		and writer.workspace.kind == "worktree"
		and writer.workspace.root ~= root
		and opened[#opened].command[2]:find("Gator runbook dependency context", 1, true),
	"writers must get isolated worktrees and explicit dependency provenance"
)
helpers.write(writer.workspace.root .. "/writer.lua", "return 'writer'\n")
value:report_usage(writer.id, { state = "reported", input_tokens = 2, output_tokens = 2, total_tokens = 4 })
value:update(writer.id, { state = "completed" })

assert(
	value:start_runbook_step(runbook.id, "review", false),
	"reviewer handoff must open only after its writer completed"
)
local reviewer
for _, candidate in ipairs(value:runs()) do
	if candidate.runbook_id == runbook.id and candidate.role == "reviewer" then
		reviewer = candidate
		break
	end
end
assert(
	review
		and reviewer
		and reviewer.workspace.root == writer.workspace.root
		and reviewer.depends_on[1] == writer.id
		and review.body:find("Gator runbook dependency context", 1, true),
	"reviewers must receive reviewed provenance in the completed writer workspace without a second writer worktree"
)
value:report_usage(reviewer.id, { state = "reported", input_tokens = 1, output_tokens = 1, total_tokens = 2 })
value:update(reviewer.id, { state = "completed" })

assert(
	not pcall(value.start_runbook_step, value, runbook.id, "integrate", false),
	"integrator convergence must require explicit user confirmation"
)
local integrator = value:start_runbook_step(runbook.id, "integrate", true)
assert(
	integrator.role == "integrator"
		and integrator.workspace.kind == "worktree"
		and integrator.workspace.root ~= writer.workspace.root
		and opened[#opened].command[2]:find("### " .. writer.id, 1, true)
		and opened[#opened].command[2]:find("### " .. reviewer.id, 1, true),
	"integrators must receive every completed dependency as provenance in a newly isolated worktree"
)
local status = value:runbook_status(runbook.id)
assert(
	status.reported_tokens == 8 and status.usage_state == "partial" and status.tracked_runs == 4,
	"runbook usage must keep an unreported provider session visibly partial rather than inventing cost"
)
value:report_usage(integrator.id, { state = "reported", input_tokens = 1, output_tokens = 0, total_tokens = 1 })
assert(
	value:runbook_status(runbook.id).reported_tokens == 9 and value:runbook_status(runbook.id).usage_state == "reported",
	"runbook usage must become exact only after every started run reports usage"
)

local budgeted = value:create_runbook({
	id = "budgeted",
	title = "Bounded research",
	max_concurrent = 1,
	max_tokens = 3,
	steps = {
		{ id = "one", role = "researcher", objective = "First", provider = "pi", depends_on = {} },
		{ id = "two", role = "researcher", objective = "Second", provider = "pi", depends_on = {} },
	},
})
assert(
	value:runbook_status(budgeted.id).usage_state == "unknown",
	"a runbook with no provider usage must report unknown rather than a fabricated zero-cost total"
)
local one = value:start_runbook_step(budgeted.id, "one", false)
assert(
	not pcall(value.start_runbook_step, value, budgeted.id, "two", false),
	"runbook concurrency must block a second ready step without scheduling it"
)
value:report_usage(one.id, { state = "reported", input_tokens = 2, output_tokens = 1, total_tokens = 3 })
value:update(one.id, { state = "completed" })
assert(
	not pcall(value.start_runbook_step, value, budgeted.id, "two", false),
	"a runbook cap must block future starts once reported usage reaches its exact lower bound"
)
