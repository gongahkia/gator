local event = require("gator").module("telemetry").event
local value = event.event({
	schema_version = 1,
	type = "error",
	at = 1,
	fields = {
		code = "indexer.failed",
		detail = "token=event-secret at /Users/example/project and C:\\Users\\example\\project",
	},
})
assert(
	value.type == "error"
		and value.fields.code == "indexer.failed"
		and not value.fields.detail:find("event%-secret")
		and not value.fields.detail:find("/Users/example/project", 1, true)
		and not value.fields.detail:find("C:\\Users\\example\\project", 1, true),
	"telemetry events must redact secrets and filesystem paths"
)
assert(not pcall(event.event, {
	schema_version = 1,
	type = "feature",
	at = 1,
	fields = { feature = "dashboard", path = "/private/project" },
}), "telemetry event schemas must reject unallowlisted fields")
assert(not pcall(event.event, {
	schema_version = 1,
	type = "unknown",
	at = 1,
	fields = {},
}), "unsupported telemetry event types must fail explicitly")
