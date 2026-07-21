local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local transport = require("gator").module("adapters").codex_transport
local fixture = vim.g.gator_test.root .. "/tests/fixtures/adapters/codex_appserver.jsonl"

local records = {}
local value = transport.new()
local report = value:replay(fixture, function(record, meta)
	records[meta.index] = meta.kind .. ":" .. (record.method or "response")
end)
assert(
	report.records == 5
		and report.requests == 2
		and report.notifications == 1
		and report.responses == 2
		and report.failures == 0,
	"Codex transport fixture must conform to the generated v2 schema subset"
)
assert(
	vim.deep_equal(records, {
		"request:initialize",
		"response:response",
		"notification:initialized",
		"request:account/read",
		"response:response",
	}),
	"Codex transport fixture must preserve native request, response, and notification order"
)

local failed = helpers.tempdir("codex-transport") .. "/failed.jsonl"
helpers.write(failed, '{"id":1,"method":"initialize","params":{"clientInfo":{"name":"gator","version":"1"}}}\n')
assert(not pcall(value.replay, value, failed), "Codex transport fixtures must reject missing responses")

local error = helpers.tempdir("codex-transport-error") .. "/error.jsonl"
helpers.write(
	error,
	'{"id":1,"method":"account/read","params":{}}\n{"id":1,"error":{"code":-32000,"message":"fixture unavailable"}}\n'
)
assert(
	transport.new():replay(error).failures == 1,
	"Codex transport fixtures must classify provider-native failure responses"
)

local unavailable = transport.unavailable("Codex token=fixture-secret is unavailable")
assert(
	unavailable:status().state == "unavailable"
		and unavailable:status().reason == "Codex token=[REDACTED] is unavailable",
	"Codex transport unavailability must be explicit and redacted"
)
assert(not pcall(unavailable.replay, unavailable, fixture), "unavailable Codex transports must reject fixture replay")

local cancelled = transport.new()
assert(cancelled:cancel("token=fixture-secret"), "ready Codex transport fixtures must support cancellation")
assert(
	cancelled:status().state == "cancelled" and cancelled:status().reason == "token=[REDACTED]",
	"Codex transport cancellation reasons must be redacted"
)
assert(not pcall(cancelled.replay, cancelled, fixture), "cancelled Codex transports must reject fixture replay")
