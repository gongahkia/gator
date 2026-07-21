local fixtures = require("gator").module("adapters").fixtures
local stream = require("gator").module("adapters").codex_stream
local fixture = vim.g.gator_test.root .. "/tests/fixtures/adapters/codex_stream.jsonl"
local value = stream.new({
	event_id = function(context)
		return "codex-event-" .. context.sequence
	end,
	now = function()
		return 7
	end,
})
local events = {}
fixtures.replay_jsonl(fixture, function(record)
	for _, event in ipairs(value:feed(record, { run_id = "run-codex", session_id = "codex-fixture" })) do
		table.insert(events, event)
	end
end)
assert(
	#events == 5
		and events[1].type == "run.started"
		and events[2].type == "message.started"
		and events[3].type == "message.delta"
		and events[3].payload.text == "gator-fixture"
		and events[4].type == "message.completed"
		and events[5].type == "run.completed"
		and events[5].payload.status == "completed",
	"Codex notifications must normalize native turn and assistant-message lifecycles"
)

local failed = stream.new()
local error = failed:feed({
	method = "error",
	params = {
		threadId = "codex-fixture",
		turnId = "turn-fixture",
		willRetry = true,
		error = { message = "token=fixture-secret" },
	},
}, { run_id = "run-codex", session_id = "codex-fixture" })[1]
assert(
	error.type == "run.error" and error.payload.message == "token=[REDACTED]" and error.payload.retryable,
	"Codex stream failures must be explicit and redacted"
)

local mismatch = stream.new()
assert(not pcall(mismatch.feed, mismatch, {
	method = "turn/started",
	params = { threadId = "other-thread", turn = { id = "turn-fixture", items = {}, status = "inProgress" } },
}, { run_id = "run-codex", session_id = "codex-fixture" }), "Codex streams must reject cross-thread native events")

local unavailable = stream.unavailable("token=fixture-secret")
assert(
	unavailable:status().state == "unavailable" and unavailable:status().reason == "token=[REDACTED]",
	"Codex stream unavailability must remain explicit and redacted"
)
assert(
	not pcall(unavailable.feed, unavailable, { id = 1, result = {} }, { run_id = "run-codex" }),
	"unavailable Codex streams must reject records"
)

local cancelled = stream.new()
assert(cancelled:cancel("token=fixture-secret"), "ready Codex streams must support cancellation")
assert(
	cancelled:status().state == "cancelled" and cancelled:status().reason == "token=[REDACTED]",
	"Codex stream cancellation reasons must be redacted"
)
assert(
	not pcall(cancelled.feed, cancelled, { id = 1, result = {} }, { run_id = "run-codex" }),
	"cancelled Codex streams must reject records"
)
