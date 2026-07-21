local classify = require("gator").module("core").process_classification
local run = require("gator").module("core").run

local function agent(id, state, session_id)
	return run.new({
		id = id,
		task_id = "task-process-classification",
		provider = session_id and { name = "codex", session_id = session_id } or { name = "codex" },
		process = { pid = 42, executable = "codex" },
		workspace = { kind = "project", root = vim.g.gator_test.root },
		state = state,
		timing = {},
		usage = {},
	})
end

local detached = classify.classify({
	run = agent("run-detached", "running", "native-detached"),
	alive = function()
		return false
	end,
})
assert(detached.status == "detached", "non-live managed processes must classify as detached")

local orphaned = classify.classify({
	run = agent("run-orphaned", "running", "native-orphaned"),
	alive = function()
		return true
	end,
	session_probe = function(reference)
		assert(reference.owner == "provider", "session probes must retain provider ownership")
		return { available = false, reason = "native session expired" }
	end,
})
assert(orphaned.status == "orphaned", "live processes without resumable native sessions must be orphaned")

local attached = classify.classify({
	run = agent("run-attached", "running", "native-attached"),
	alive = function(pid)
		return pid == 42
	end,
	session_probe = function()
		return true
	end,
})
assert(
	attached.status == "attached" and attached.session.id == "native-attached",
	"live processes with provider-owned sessions must remain attached"
)

assert(
	classify.classify({ run = agent("run-cancelled", "cancelled", "native-cancelled") }).status == "cancelled",
	"cancelled processes must remain explicit"
)
assert(
	classify.classify({ run = agent("run-unavailable", "running", "native-unavailable") }).status == "unavailable",
	"missing liveness probes must remain explicit"
)

local failed = classify.classify({
	run = agent("run-probe-failed", "running", "native-failed"),
	alive = function()
		error("token: private-value")
	end,
})
assert(
	failed.status == "failed" and not failed.reason:find("private%-value"),
	"process-probe failures must be explicit and redacted"
)
