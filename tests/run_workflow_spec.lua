local config = require("gator.config")
local state = require("gator.state")
local workflow = require("gator.workflow")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local root = helpers.tempdir("run-workflow")
assert(
	vim.system({ "git", "init", "-q" }, { cwd = root, text = true }):wait().code == 0,
	"test project must initialize Git"
)
helpers.write(root .. "/baseline.lua", "return 'baseline'\n")
assert(vim.system({ "git", "add", "baseline.lua" }, { cwd = root, text = true }):wait().code == 0 and vim.system({
	"git",
	"-c",
	"user.name=Gator",
	"-c",
	"user.email=gator@example.invalid",
	"commit",
	"-qm",
	"baseline",
}, { cwd = root, text = true })
	:wait().code == 0, "test project must have a worktree base revision")

vim.cmd("enew!")
local buffer = vim.api.nvim_get_current_buf()
vim.api.nvim_buf_set_lines(buffer, 0, -1, false, { "local selected = true", "return selected" })
local selection = require("gator.context.capture").current({ buffer = buffer, first_line = 1, last_line = 1 })
local whole_buffer = require("gator.context.capture").current({ buffer = buffer })
assert(whole_buffer.first_line == 1 and whole_buffer.last_line == 2, "no range must capture the current buffer")
local opened = {}
local loading_events = {}
local reviewed = nil
local current = state.new(
	config.resolve({
		providers = { pi = { user_confirmed = true } },
		budget = { max_tokens = 5, action = "warn" },
	}),
	{ supported = true }
)
local value = workflow.new({
	state = current,
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
		resume = function(_, opts, callback)
			callback({ session = opts.session, command = { "pi", "--session", opts.session.id } })
		end,
	},
	terminal = {
		open = function(_, opts)
			table.insert(opened, opts)
			return { job_id = 1 }
		end,
		attach = function()
			return true
		end,
		close = function()
			return true
		end,
	},
	loading = {
		open = function(opts)
			table.insert(loading_events, opts.message)
			return {
				close = function()
					table.insert(loading_events, "closed")
				end,
			}
		end,
	},
	handoff_review = {
		open = function(opts)
			reviewed = opts
			opts.on_confirm(opts.body)
		end,
	},
})

local run =
	value:launch({ provider = "pi", transport = "terminal", objective = "Fix selected code", capture = selection })
local persisted = value:run(run.id)
assert(
	persisted.state == "running"
		and persisted.transport == "terminal"
		and persisted.transcript == "unavailable"
		and persisted.usage.state == "unknown"
		and persisted.usage.context_tokens_estimate > 0
		and persisted.budget.state == "unknown",
	"launch must create a run-first terminal record with explicit transcript and usage state"
)
assert(
	#opened == 1 and opened[1].command[2]:find("local selected = true", 1, true),
	"native launch must receive the captured source rather than only the objective"
)
assert(
	vim.fn.filereadable(root .. "/.gator/runs/" .. run.id .. ".json") == 1
		and table.concat(vim.fn.readfile(root .. "/.git/info/exclude"), "\n"):find(".gator/", 1, true),
	"run records must be project-local and locally ignored"
)
assert(value.store:project().default_provider == "pi", "successful launch must remember the project provider")
assert(
	loading_events[1] == "Starting pi" and loading_events[2] == "closed",
	"launch preparation must close its loading dialog once the provider bridge is ready"
)
local isolated =
	value:launch({ provider = "pi", transport = "terminal", objective = "Work concurrently", capture = selection })
assert(
	isolated.workspace.kind == "worktree"
		and isolated.workspace.root ~= root
		and vim.fn.isdirectory(isolated.workspace.root) == 1
		and opened[#opened].cwd == isolated.workspace.root,
	"a concurrent writer must launch in its own Git worktree"
)
assert(value:stop(isolated.id), "test concurrent writer must stop")
assert(value:resume(run.id), "a live terminal must focus its existing Neovim terminal before relaunching")
assert(value:stop(run.id) and value:run(run.id).state == "stopped", "stopping must update the run graph state")
helpers.write(root .. "/handoff-source.lua", "return 'handoff snapshot'\n")
assert(value:handoff(run.id, "pi"), "reviewed handoffs must accept a target provider")
local handoff_run
for _, candidate in ipairs(value:runs()) do
	if candidate.parent_run_id == run.id then
		handoff_run = candidate
		break
	end
end
assert(
	reviewed
		and reviewed.body:find("Handoff file snapshot", 1, true)
		and handoff_run
		and vim.fn.filereadable(root .. "/.gator/handoffs/" .. handoff_run.bundle_id .. "/files/handoff-source.lua")
			== 1,
	"reviewed handoffs must capture and materialize live source-workspace file context"
)
assert(value:stop(handoff_run.id), "test handoff run must stop before the next isolated launch")
local handoff = value:launch({
	provider = "pi",
	transport = "terminal",
	objective = "Continue reviewed work",
	bundle_body = "# Reviewed handoff",
	handoff_snapshot = { files = { { path = "handoff.lua", state = "included", content = "return true\n" } } },
	remember = false,
})
assert(
	vim.fn.filereadable(root .. "/.gator/handoffs/" .. handoff.bundle_id .. "/files/handoff.lua") == 1
		and opened[#opened].command[2]:find(".gator/handoffs/" .. handoff.bundle_id .. "/files/", 1, true),
	"handoff launches must materialize reviewed source-file snapshots for the target agent"
)
value:report_usage(handoff.id, { state = "reported", input_tokens = 2, output_tokens = 3, total_tokens = 5 })
assert(
	value:run(handoff.id).usage.state == "reported" and value:run(handoff.id).budget.state == "exhausted",
	"reported provider usage must replace estimates and update the configured budget state"
)
local cancelled = false
value:update(handoff.id, { budget = { limit_tokens = 5, action = "stop", state = "unknown" } })
value.active[handoff.id] =
	{ kind = "structured", handle = {
		cancel = function()
			cancelled = true
			return true
		end,
	} }
value:report_usage(handoff.id, { state = "reported", input_tokens = 2, output_tokens = 3, total_tokens = 5 })
assert(cancelled, "stop budgets must cancel an active structured run when reported usage reaches the limit")
