local actions = require("gator").module("review").actions
local task = require("gator").module("core").task

local reviewed = task.new({
	id = "task-action",
	objective = "Exercise review actions",
	lifecycle = "awaiting_review",
	evidence = { { kind = "test", ref = "test://unit" } },
	created_at = 1,
	updated_at = 2,
})
local accepted = actions.accept({ task = reviewed, reviewer = "alice", at = 3 })
assert(
	accepted.task.lifecycle == "merged" and accepted.task.evidence[2].kind == "review-action",
	"accept must record reversible merge evidence"
)
local restored = actions.undo({ task = accepted.task, receipt = accepted.receipt, at = 4 })
assert(
	restored.lifecycle == "awaiting_review" and #restored.evidence == 1,
	"undo must restore local task evidence and lifecycle"
)
local rejected = actions.reject({ task = restored, reviewer = "alice", at = 5 })
assert(rejected.task.lifecycle == "discarded", "reject must discard an awaiting-review task")
assert(
	not pcall(actions.accept, { task = rejected.task, reviewer = "alice", at = 6 }),
	"terminal decisions must not be reapplied"
)
