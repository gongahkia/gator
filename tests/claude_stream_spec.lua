local fixtures = require("gator").module("adapters").fixtures
local stream = require("gator").module("adapters").claude_stream
local fixture = vim.g.gator_test.root .. "/tests/fixtures/adapters/claude_stream.jsonl"
local value = stream.new({
	event_id = function(context)
		return "claude-event-" .. context.sequence
	end,
	now = function()
		return 7
	end,
})
local events = {}
fixtures.replay_jsonl(fixture, function(record)
	for _, event in ipairs(value:feed(record, { run_id = "run-claude", session_id = "claude-fixture" })) do
		table.insert(events, event)
	end
end)
assert(
	#events == 3
		and events[1].type == "run.started"
		and events[2].type == "message.completed"
		and events[2].payload.text == "hello"
		and events[3].type == "run.completed"
		and events[3].payload.status == "completed",
	"Claude streams must normalize native initialization, assistant text, and completion"
)

local signals = {}
fixtures.replay_jsonl(vim.g.gator_test.root .. "/tests/fixtures/adapters/claude_signals.jsonl", function(record)
	for _, event in ipairs(value:feed(record, { run_id = "run-claude", session_id = "claude-fixture" })) do
		table.insert(signals, event)
	end
end)
assert(
	signals[1].type == "usage.update"
		and signals[1].payload.input == 2
		and signals[1].payload.total == 5
		and signals[2].type == "run.completed"
		and signals[3].type == "file.change"
		and signals[3].payload.path == "lua/gator/init.lua"
		and signals[4].type == "context.compacted"
		and signals[4].payload.before == 5,
	"Claude stream records must normalize usage, persisted files, and compaction signals"
)

local unsafe_file = stream.new()
assert(not pcall(unsafe_file.feed, unsafe_file, {
	type = "system",
	subtype = "files_persisted",
	session_id = "claude-fixture",
	files = { { filename = "../secret", file_id = "file-fixture" } },
	failed = {},
	processed_at = "2026-07-21T00:00:00Z",
}, { run_id = "run-claude", session_id = "claude-fixture" }), "Claude streams must reject unsafe persisted file paths")

local failed = stream.new()
local error = failed:feed({
	type = "result",
	subtype = "error",
	is_error = true,
	session_id = "claude-fixture",
	result = "token=fixture-secret",
}, { run_id = "run-claude", session_id = "claude-fixture" })[1]
assert(
	error.type == "run.error" and error.payload.message == "token=[REDACTED]" and not error.payload.retryable,
	"Claude stream failures must be explicit and redacted"
)

local mismatch = stream.new()
assert(not pcall(mismatch.feed, mismatch, {
	type = "system",
	subtype = "init",
	session_id = "other-session",
}, { run_id = "run-claude", session_id = "claude-fixture" }), "Claude streams must reject cross-session native events")

local unavailable = stream.unavailable("token=fixture-secret")
assert(
	unavailable:status().state == "unavailable" and unavailable:status().reason == "token=[REDACTED]",
	"Claude stream unavailability must remain explicit and redacted"
)
assert(not pcall(unavailable.feed, unavailable, { type = "system", subtype = "init", session_id = "claude-fixture" }, {
	run_id = "run-claude",
}), "unavailable Claude streams must reject records")

local cancelled = stream.new()
assert(cancelled:cancel("token=fixture-secret"), "ready Claude streams must support cancellation")
assert(
	cancelled:status().state == "cancelled" and cancelled:status().reason == "token=[REDACTED]",
	"Claude stream cancellation reasons must be redacted"
)
