local core = require("gator").module("core")
local task = core.task
local session = core.session
local entity = task.new({
	id = "task-session",
	objective = "Link a native provider session",
	created_at = 1,
	updated_at = 1,
})
local native = session.new({
	task_id = "task-session",
	provider = "codex",
	id = "native-session-1",
	owner = "provider",
})

assert(session.is(native), "provider-native session links must be typed")
local linked = session.link(entity, native, 2)
assert(#linked.sessions == 1, "session links must be attached to the task")
assert(linked.sessions[1].id == "native-session-1", "native identifiers must remain opaque")
assert(linked.updated_at == 2, "new links must update task persistence time")
assert(#entity.sessions == 0, "linking must not mutate the original task")
assert(#session.link(linked, native).sessions == 1, "linking an existing session must remain idempotent")

local wrong_task = session.new({
	task_id = "task-other",
	provider = "codex",
	id = "native-session-2",
	owner = "provider",
})
local ok = pcall(session.link, entity, wrong_task)
assert(not ok, "sessions must not link across tasks")
ok = pcall(session.new, {
	task_id = "task-session",
	provider = "codex",
	id = "native-session-3",
	owner = "gator",
})
assert(not ok, "session ownership must remain provider-native")
ok = pcall(session.new, {
	task_id = "task-session",
	provider = "codex",
	id = "native-session-4",
	owner = "provider",
	history = "rewritten transcript",
})
assert(not ok, "session links must reject rewritten provider history")
