local config = require("gator.config")
local state = require("gator.state")
local workflow = require("gator.workflow")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local root = helpers.tempdir("workflow-review")
assert(vim.system({ "git", "init", "-q" }, { cwd = root, text = true }):wait().code == 0, "fixture must initialize Git")
helpers.write(root .. "/review.lua", "return 'baseline'\n")
assert(
	vim.system({ "git", "add", "review.lua" }, { cwd = root, text = true }):wait().code == 0,
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
helpers.write(root .. "/review.lua", "return 'changed'\n")

local opened = nil
local value = workflow.new({
	state = state.new(
		config.resolve({
			review = { commands = { unit = { argv = { "sh", "-c", "printf review-output" } } } },
			providers = { pi = { user_confirmed = true } },
		}),
		{ supported = true }
	),
	root = root,
	readiness = function()
		return { { provider = "pi", available = true, readiness_state = "user_confirmed" } }
	end,
	review_ui = {
		open = function(opts)
			opened = opts
			return true
		end,
		close = function()
			return true
		end,
	},
})
value:put({
	id = "run-review",
	provider = "pi",
	role = "writer",
	transport = "chat",
	state = "completed",
	workspace = { kind = "project", root = root },
	bundle_id = "bundle-review",
	objective = "Review worktree changes",
	transcript = "available",
	usage = { state = "unknown", context_tokens_estimate = 1 },
	budget = { limit_tokens = 0, action = "warn", state = "unbounded" },
	session = { id = "pi-review", resume_supported = true },
	created_at = 1,
	updated_at = 1,
})

assert(value:review("run-review"), "review must open against a Gator run worktree")
assert(
	opened
		and opened.base_sha ~= ""
		and opened.diff:find("changed", 1, true)
		and vim.deep_equal(opened.commands, { "unit" }),
	"review must show the real worktree diff, base SHA, and explicitly configured test commands"
)
assert(
	not pcall(value.record_review_decision, value, "run-review", "accepted"),
	"a configured review command must produce matching passed evidence before acceptance"
)
local evidence = value:execute_review_test("run-review", "unit", true)
assert(
	evidence.review.state == "passed"
		and evidence.review.exit_code == 0
		and helpers.read(root .. "/.gator/" .. evidence.output_ref):find("review-output", 1, true)
		and value:run("run-review").review.state == "passed",
	"approved tests must preserve command outcome, output reference, and review state"
)
assert(
	value.store:list_reviews("run-review")[1].diff_sha256 == opened.diff_sha256,
	"review evidence must retain the reviewed diff hash"
)
assert(
	not pcall(value.execute_review_test, value, "run-review", "unit", false),
	"review tests must require explicit approval"
)
assert(
	value:record_review_decision("run-review", "accepted").review.state == "accepted",
	"review decisions must be immutable evidence"
)
helpers.write(root .. "/review.lua", "return 'stale'\n")
assert(
	not pcall(value.execute_review_test, value, "run-review", "unit", true),
	"review tests must refuse a changed target worktree after the displayed diff was reviewed"
)
