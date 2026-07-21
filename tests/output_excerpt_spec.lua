local excerpts = require("gator").module("core").output_excerpt

local value = excerpts.new({ limit = 20 })
value:append("first-output-")
local bounded = value:append("token: private-value")
assert(
	bounded.state == "ready"
		and bounded.bytes <= 20
		and bounded.truncated
		and bounded.dropped_bytes > 0
		and not bounded.text:find("private%-value"),
	"live output must retain only bounded, redacted excerpts"
)

assert(value:cancel("user cancelled"), "output excerpts must support explicit cancellation")
assert(
	value:append("later output").state == "cancelled" and not value:cancel(),
	"cancelled excerpts must reject later provider output"
)

local unavailable = excerpts.unavailable({ reason = "provider streaming is unavailable", limit = 8 })
assert(
	not unavailable:status().available and unavailable:append("output").state == "unavailable",
	"unavailable output excerpts must remain explicit"
)

local failed = excerpts.new({ limit = 8 }):append({})
assert(
	failed.state == "failed" and failed.reason == "chunk must be non-empty text",
	"invalid live output must become an explicit excerpt failure"
)
