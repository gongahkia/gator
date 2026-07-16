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

local planned = lifecycle.transition(draft, "planned", 2)
assert(planned.lifecycle == "planned", "valid transitions must update lifecycle")
assert(draft.lifecycle == "draft", "transitions must not mutate prior task entities")
local running = lifecycle.transition(planned, "running", 3)
local review = lifecycle.transition(running, "awaiting_review", 4)
local merged = lifecycle.transition(review, "merged", 5)
assert(merged.lifecycle == "merged" and merged.updated_at == 5, "reviewed tasks must merge with updated evidence time")

local failed = lifecycle.transition(running, "failed", 4)
assert(lifecycle.transition(failed, "planned", 5).lifecycle == "planned", "failed tasks must support planned retry")

local ok = pcall(lifecycle.transition, draft, "merged", 2)
assert(not ok, "invalid lifecycle skips must fail explicitly")
ok = pcall(lifecycle.transition, merged, "planned", 6)
assert(not ok, "terminal lifecycle states must reject transitions")
ok = pcall(lifecycle.transition, planned, "running", 1)
assert(not ok, "transition timestamps must be monotonic")
