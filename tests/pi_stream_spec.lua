local fixtures = require("gator").module("adapters").fixtures
local stream = require("gator").module("adapters").pi_stream
local fixture = vim.g.gator_test.root .. "/tests/fixtures/adapters/pi_rpc.jsonl"
local value = stream.new({
	event_id = function(context)
		return "pi-event-" .. context.sequence
	end,
	now = function()
		return 7
	end,
})
local events = {}
fixtures.replay_jsonl(fixture, function(record)
	for _, event in ipairs(value:feed(record, { run_id = "run-pi", session_id = "pi-fixture" })) do
		table.insert(events, event)
	end
end)
assert(
	#events == 2
		and events[1].id == "pi-event-0"
		and events[1].type == "message.completed"
		and events[1].payload.text == "gator-fixture"
		and events[1].provider.session_id == "pi-fixture"
		and events[2].type == "run.completed"
		and events[2].payload.message_count == 1,
	"Pi stream records must normalize assistant and lifecycle events"
)

local protected = stream.new()
local redacted = protected:feed({
	type = "message_end",
	message = { role = "assistant", content = { { type = "text", text = "token=fixture-secret" } } },
}, { run_id = "run-pi" })[1]
assert(
	redacted.payload.text == "token=[REDACTED]",
	"Pi stream messages must apply configured redaction before normalization"
)

local invalid = stream.new()
assert(
	not pcall(
		invalid.feed,
		invalid,
		{ type = "message_end", message = { role = "assistant", content = { { type = "text" } } } },
		{ run_id = "run-pi" }
	),
	"Pi streams must reject malformed assistant message records"
)

local unavailable = stream.unavailable("token=fixture-secret")
assert(
	unavailable:status().state == "unavailable" and unavailable:status().reason == "token=[REDACTED]",
	"Pi stream unavailability must be explicit and redacted"
)
assert(
	not pcall(unavailable.feed, unavailable, { type = "response" }, { run_id = "run-pi" }),
	"unavailable Pi streams must reject records"
)

local cancelled = stream.new()
assert(cancelled:cancel("token=fixture-secret"), "ready Pi streams must support cancellation")
assert(
	cancelled:status().state == "cancelled" and cancelled:status().reason == "token=[REDACTED]",
	"Pi stream cancellation must redact reasons"
)
assert(
	not pcall(cancelled.feed, cancelled, { type = "response" }, { run_id = "run-pi" }),
	"cancelled Pi streams must reject records"
)
