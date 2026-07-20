local core = require("gator").module("core")
local task = core.task
local lifecycle = core.lifecycle
local draft = task.new({
	id = "task-lifecycle",
	objective = "Exercise lifecycle transitions",
	created_at = 1,
	updated_at = 1,
})

assert(lifecycle.can_transition("draft", "planned"), "draft tasks must be plannable")
assert(not lifecycle.can_transition("draft", "merged"), "draft tasks must not skip directly to merged")
assert(
	vim.deep_equal(
		lifecycle.states(),
		{ "draft", "planned", "running", "awaiting_review", "failed", "merged", "discarded" }
	),
	"lifecycle must expose its canonical state vocabulary"
)
assert(
	vim.deep_equal(lifecycle.next_states("running"), { "awaiting_review", "failed", "discarded" }),
	"lifecycle must expose deterministic valid successor states"
)

local vocabulary = lifecycle.states()
vocabulary[1] = "modified"
assert(lifecycle.states()[1] == "draft", "lifecycle vocabulary must not expose mutable internals")

local planned = lifecycle.transition(draft, "planned", 2)
assert(planned.lifecycle == "planned", "valid transitions must update lifecycle")
assert(draft.lifecycle == "draft", "transitions must not mutate prior task entities")
local running = lifecycle.transition(planned, "running", 3)
local review = lifecycle.transition(running, "awaiting_review", 4)
local merged = lifecycle.transition(review, "merged", 5)
assert(merged.lifecycle == "merged" and merged.updated_at == 5, "reviewed tasks must merge with updated evidence time")

local failed = lifecycle.transition(running, "failed", 4)
assert(lifecycle.transition(failed, "planned", 5).lifecycle == "planned", "failed tasks must support planned retry")
assert(lifecycle.cancel(running, 4).lifecycle == "discarded", "running tasks must support explicit cancellation")

local ok = pcall(lifecycle.transition, draft, "merged", 2)
assert(not ok, "invalid lifecycle skips must fail explicitly")
ok = pcall(lifecycle.transition, merged, "planned", 6)
assert(not ok, "terminal lifecycle states must reject transitions")
ok = pcall(lifecycle.transition, planned, "running", 1)
assert(not ok, "transition timestamps must be monotonic")
ok = pcall(lifecycle.next_states, "missing")
assert(not ok, "unknown lifecycle states must fail explicitly")
ok = pcall(lifecycle.cancel, merged, 6)
assert(not ok, "terminal tasks must reject cancellation")
