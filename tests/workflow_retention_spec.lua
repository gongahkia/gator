local config = require("gator.config")
local state = require("gator.state")
local workflow = require("gator.workflow")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local day = 24 * 60 * 60
local root = helpers.tempdir("workflow-retention")
assert(vim.system({ "git", "init", "-q" }, { cwd = root, text = true }):wait().code == 0, "fixture must initialize Git")
local value = workflow.new({
	state = state.new(config.resolve({ retention = { max_age_days = 1 } }), { supported = true }),
	root = root,
	clock = function()
		return 2 * day
	end,
	readiness = function()
		return {}
	end,
})
value:put({
	id = "run-expired",
	provider = "pi",
	role = "primary",
	transport = "terminal",
	state = "completed",
	workspace = { kind = "project", root = root },
	bundle_id = "bundle-expired",
	objective = "Expired artifact",
	transcript = "unavailable",
	usage = { state = "unknown", context_tokens_estimate = 1 },
	budget = { limit_tokens = 0, action = "warn", state = "unbounded" },
	created_at = 1,
	updated_at = 1,
})
value.store:bundle("bundle-expired", "old bundle")
value.store:transcript("run-expired", "old transcript")
for _, path in ipairs({
	root .. "/.gator/runs/run-expired.json",
	root .. "/.gator/bundles/bundle-expired.md",
	root .. "/.gator/transcripts/run-expired.md",
}) do
	vim.uv.fs_utime(path, 1, 1)
end
assert(
	value:recover() == 0 and value.store:get("run-expired") == nil,
	"startup retention must automatically remove expired managed runs"
)
assert(
	vim.fn.filereadable(root .. "/.gator/bundles/bundle-expired.md") == 0
		and vim.fn.filereadable(root .. "/.gator/transcripts/run-expired.md") == 0,
	"startup retention must remove only expired managed run artifacts"
)
