local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local evidence = require("gator").module("review").evidence

local path = helpers.tempdir("review-evidence") .. "/evidence.json"
local store = evidence.open(path)
store:record({ task_id = "task-evidence", kind = "reviewer", reviewer = "alice" })
store:record({
	task_id = "task-evidence",
	kind = "test",
	command_id = "unit",
	output_ref = "test://unit-1",
	passed = true,
	at = 1,
})
store:record({ task_id = "task-evidence", kind = "approval", reviewer = "alice", approved = true, at = 2 })
store:record({
	task_id = "task-evidence",
	kind = "revision",
	base_revision = "base-1",
	head_revision = "head-1",
	at = 3,
})
local restored = evidence.open(path):task("task-evidence")
assert(
	restored.reviewers[1] == "alice"
		and restored.tests[1].output_ref == "test://unit-1"
		and restored.approvals[1].approved
		and restored.revisions[1].head_revision == "head-1",
	"review and test evidence must persist with the task"
)
assert(not pcall(store.record, store, {
	task_id = "task-evidence",
	kind = "test",
	command_id = "unit",
	output_ref = "test://unit-2",
	passed = true,
	credential = "secret",
}), "review evidence must reject credential fields")
