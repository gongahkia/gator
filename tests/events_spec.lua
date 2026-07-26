local events = require("gator").module("extensions").events
local received = {}
local run_listener = events.subscribe({
	types = { "run.created" },
	handler = function(value)
		table.insert(received, value)
		value.payload.run.id = "changed"
	end,
})
local all_listener = events.subscribe({
	handler = function(value)
		assert(value.schema_version == 1, "events must expose their schema version")
	end,
})
local emitted = events.emit({
	schema_version = 1,
	type = "run.created",
	at = 1,
	payload = {
		run = { id = "run-one" },
		session = { provider = "fixture", id = "native-session", owner = "provider" },
	},
})
assert(
	#received == 1 and received[1].type == "run.created" and emitted.payload.run.id == "run-one",
	"event delivery must be versioned, filtered, and isolated from listeners"
)
assert(events.unsubscribe(run_listener) and events.unsubscribe(all_listener), "listeners must unsubscribe cleanly")
assert(not events.unsubscribe(run_listener), "unknown listeners must not be silently removed")
assert(not pcall(events.emit, {
	schema_version = 1,
	type = "run.created",
	at = 1,
	payload = { token = "secret" },
}), "events must reject credentials")
assert(not pcall(events.event, {
	schema_version = 1,
	type = "session.linked",
	at = 1,
	payload = { session = { provider = "fixture", id = "native-session", owner = "gator" } },
}), "session events must preserve provider ownership")
assert(not pcall(events.subscribe, {
	types = { "unknown.created" },
	handler = function() end,
}), "unsupported event domains must fail explicitly")
local failed = events.subscribe({
	types = { "run.started" },
	handler = function()
		error("fixture failure")
	end,
})
assert(not pcall(events.emit, { schema_version = 1, type = "run.started", at = 2 }), "listener failures must surface")
assert(events.unsubscribe(failed), "failed listeners must remain removable")
