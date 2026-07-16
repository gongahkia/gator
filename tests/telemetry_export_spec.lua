local export = require("gator").module("telemetry").export
local consent = require("gator").module("telemetry").consent
local calls = {}
local transport = export.open({
	endpoint = "https://telemetry.example.test/v1/events",
	consent = consent.new({ enabled = true }),
	max_attempts = 3,
	send = function(request)
		table.insert(calls, request)
		return { code = #calls == 1 and 503 or 204 }
	end,
})
local snapshot = {
	schema_version = 1,
	features = { { feature = "dashboard", count = 2 } },
	errors = { { code = "indexer.failed", count = 1 } },
	performance = { { operation = "context.search", count = 1, duration_ms = 12 } },
	health = { { component = "indexer", status = "degraded", count = 1 } },
}
local result = transport:send(snapshot)
assert(
	result.sent and result.attempts == 2 and #calls == 2 and calls[1].body:find("dashboard", 1, true),
	"telemetry export must retry validated aggregate payloads"
)
assert(
	transport:inspect().last.sent and transport:disable() and not pcall(transport.send, transport, snapshot),
	"telemetry export must support inspect and explicit runtime disable controls"
)
assert(not pcall(export.open, {
	endpoint = "http://telemetry.example.test",
	consent = consent.new({ enabled = true }),
}), "telemetry export must require credential-free HTTPS endpoints")
local denied = export.open({
	endpoint = "https://telemetry.example.test",
	consent = consent.new({ enabled = false }),
})
assert(not pcall(denied.send, denied, snapshot), "telemetry export must require transmission consent")
