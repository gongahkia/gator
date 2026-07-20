local event = require("gator").module("core").provider_event

local value = event.new({
	schema_version = 1,
	id = "event-provider-one",
	run_id = "run-provider-one",
	provider = { name = "codex", session_id = "native-one" },
	sequence = 0,
	type = "message.delta",
	at = 1,
	payload = { text = "token: private-value", session = { provider = "codex", id = "native-one", owner = "provider" } },
})

assert(event.is(value), "provider events must expose a distinct envelope type")
assert(
	value.schema_version == 1
		and value.provider.session_id == "native-one"
		and value.payload.text:find("private%-value") == nil,
	"provider envelopes must retain opaque native session identity and redact payload text"
)
local record = event.to_record(value)
record.payload.session.id = "changed"
assert(
	not event.is(record)
		and value.payload.session.id == "native-one"
		and event.from_record(event.to_record(value)).type == "message.delta",
	"provider event records must be isolated and round-trip safely"
)
assert(not pcall(event.new, {
	schema_version = 1,
	id = "event-provider-two",
	run_id = "run-provider-one",
	provider = { name = "codex" },
	sequence = 1,
	type = "unknown.delta",
	at = 2,
}), "provider events must reject unsupported domains")
assert(not pcall(event.new, {
	schema_version = 1,
	id = "event-provider-three",
	run_id = "run-provider-one",
	provider = { name = "codex" },
	sequence = -1,
	type = "run.cancelled",
	at = 3,
}), "provider events must reject invalid sequence values")
assert(not pcall(event.new, {
	schema_version = 1,
	id = "event-provider-four",
	run_id = "run-provider-one",
	provider = { name = "codex" },
	sequence = 2,
	type = "permission.requested",
	at = 4,
	payload = { session = { provider = "codex", id = "native-one", owner = "gator" } },
}), "provider events must reject non-provider-owned session references")
