local task = require("gator").module("core").task
local entity = task.new({
	id = "task-001",
	objective = "Add deterministic task persistence",
	lifecycle = "planned",
	workspace = { kind = "worktree", root = "/tmp/gator-task" },
	sessions = { { provider = "codex", id = "native-session-1", owner = "provider" } },
	evidence = { { kind = "test", ref = "make check" } },
	created_at = 1,
	updated_at = 2,
})

assert(task.is(entity), "tasks must have a distinct entity type")
assert(entity.lifecycle == "planned", "tasks must preserve lifecycle")
assert(entity.workspace.kind == "worktree", "tasks must preserve workspace references")
assert(entity.sessions[1].owner == "provider", "tasks must preserve provider session ownership")
assert(entity.evidence[1].ref == "make check", "tasks must preserve evidence references")

local record = task.to_record(entity)
assert(not task.is(record), "persistent task records must be plain tables")
record.sessions[1].id = "modified"
assert(entity.sessions[1].id == "native-session-1", "task records must not mutate entities")
assert(task.from_record(task.to_record(entity)).id == entity.id, "task records must round-trip")
assert(task.new({
	id = "task-redacted",
	objective = "token: private-value",
	evidence = { { kind = "test", ref = "ghp_private" } },
}).objective
	:find("private%-value") == nil, "task content must redact before becoming persistable")

local ok = pcall(task.new, { id = "Task-001", objective = "invalid" })
assert(not ok, "task identifiers must be stable lowercase identifiers")
ok = pcall(task.new, { id = "task-002", objective = "invalid", lifecycle = "unknown" })
assert(not ok, "unknown lifecycles must fail explicitly")
ok = pcall(task.new, {
	id = "task-003",
	objective = "invalid",
	sessions = { { provider = "codex", id = "native", owner = "gator" } },
})
assert(not ok, "tasks must reject non-provider-owned sessions")
ok = pcall(task.new, {
	id = "task-004",
	objective = "invalid",
	sessions = { { provider = "codex", id = "native", owner = "provider", token = "credential" } },
})
assert(not ok, "tasks must reject provider credential fields")
