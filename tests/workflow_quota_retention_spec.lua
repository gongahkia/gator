local config = require("gator.config")
local state = require("gator.state")
local workflow = require("gator.workflow")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local root = helpers.tempdir("workflow-quota-retention")
assert(vim.system({ "git", "init", "-q" }, { cwd = root, text = true }):wait().code == 0, "fixture must initialize Git")
local value = workflow.new({
	state = state.new(
		config.resolve({ retention = { max_age_days = 0, max_bytes = 1, cleanup_on_start = true } }),
		{ supported = true }
	),
	root = root,
	clock = function()
		return 100
	end,
	readiness = function()
		return {}
	end,
})
value:put({
	id = "run-quota",
	provider = "pi",
	role = "primary",
	transport = "terminal",
	state = "completed",
	workspace = { kind = "project", root = root },
	bundle_id = "bundle-quota",
	objective = "Retain only after preview",
	transcript = "unavailable",
	usage = { state = "unknown", context_tokens_estimate = 1 },
	budget = { limit_tokens = 0, action = "warn", state = "unbounded" },
	created_at = 1,
	updated_at = 1,
})
value.store:bundle("bundle-quota", string.rep("x", 128))
value.store:append_event("run-quota", "run.created", { role = "primary" }, 1)
local before = value:retention_inventory().bytes
assert(
	value:recover() == 0 and value:retention_inventory().bytes == before,
	"startup cleanup must not apply quota deletion"
)
local plan = value:retention_plan()
assert(
	plan.inventory.bytes == before
		and plan.quota_bytes == 1
		and #plan.artifacts > 0
		and plan.artifacts[1].reason == "quota",
	"explicit prune must show oldest-first quota candidates and inventory"
)
local applied = value:apply_retention_plan(plan)
assert(
	applied.reclaimed_bytes > 0 and value:retention_inventory().bytes <= 1,
	"quota artifacts must be removed only after explicit retention-plan application"
)
