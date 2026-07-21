local event = require("gator").module("core").provider_event
local status = require("gator").module("core").provider_status

local started = event.new({
	schema_version = 1,
	id = "event-status-started",
	run_id = "run-status",
	provider = { name = "codex", session_id = "native-status" },
	sequence = 0,
	type = "run.started",
	at = 1,
})
local completed = event.new({
	schema_version = 1,
	id = "event-status-completed",
	run_id = "run-status",
	provider = { name = "codex" },
	sequence = 1,
	type = "run.complete",
	at = 2,
})
assert(
	status.normalize("canceled") == "cancelled"
		and status.from_event(started, "queued") == "running"
		and status.from_event(completed, "running") == "completed",
	"provider statuses must normalize aliases and valid terminal transitions"
)
assert(not pcall(status.transition, "completed", "running") and not pcall(
	status.from_event,
	event.new({
		schema_version = 1,
		id = "event-status-message",
		run_id = "run-status",
		provider = { name = "codex" },
		sequence = 2,
		type = "message.delta",
		at = 3,
	}),
	"running"
), "provider statuses must reject terminal regressions and non-status events")
