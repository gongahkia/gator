local fixtures = require("gator").module("adapters").fixtures
local stream = require("gator").module("adapters").gemini_stream
local value = stream.new({
	event_id = function(context)
		return "gemini-event-" .. context.sequence
	end,
	now = function()
		return 7
	end,
})
local events = {}
fixtures.replay_jsonl(vim.g.gator_test.root .. "/tests/fixtures/adapters/gemini_stream.jsonl", function(record)
	for _, event in ipairs(value:feed(record, { run_id = "run-gemini", session_id = "gemini-fixture" })) do
		table.insert(events, event)
	end
end)
assert(
	#events == 6
		and events[1].type == "run.started"
		and events[2].type == "tool.call"
		and events[2].payload.name == "list_directory"
		and events[3].type == "tool.result"
		and events[3].payload.state == "completed"
		and events[4].type == "message.delta"
		and events[4].payload.text == "README.md"
		and events[5].type == "usage.update"
		and events[5].payload.input == 2
		and events[5].payload.total == 5
		and events[6].type == "run.completed",
	"Gemini streams must normalize native init, tool, assistant, and completion events"
)

local signals = stream.new()
signals:feed({
	type = "tool_use",
	tool_id = "write-fixture",
	tool_name = "write_file",
	parameters = { file_path = "lua/gator/init.lua", content = "fixture" },
}, { run_id = "run-gemini", session_id = "gemini-fixture" })
local file_events = signals:feed({ type = "tool_result", tool_id = "write-fixture", status = "success" }, {
	run_id = "run-gemini",
	session_id = "gemini-fixture",
})
assert(
	file_events[1].type == "tool.result"
		and file_events[2].type == "file.change"
		and file_events[2].payload.path == "lua/gator/init.lua"
		and file_events[2].payload.kind == "modified",
	"Gemini successful write tools must normalize file-change signals"
)
assert(
	stream.signals().usage.available
		and stream.signals().file_changes.available
		and not stream.signals().compaction.available,
	"Gemini signal support must expose unavailable compaction explicitly"
)

local failed = stream.new()
local error_events = failed:feed({
	type = "result",
	status = "error",
	error = { type = "provider", message = "token=fixture-secret" },
	stats = {
		total_tokens = 3,
		input_tokens = 2,
		output_tokens = 1,
		cached = 0,
		input = 2,
		duration_ms = 1,
		tool_calls = 0,
	},
}, {
	run_id = "run-gemini",
	session_id = "gemini-fixture",
})
local error = error_events[2]
assert(
	error_events[1].type == "usage.update"
		and error.type == "run.error"
		and error.payload.message == "token=[REDACTED]"
		and not error.payload.retryable,
	"Gemini stream failures must remain explicit and redacted"
)

local warning = stream.new():feed({ type = "error", message = "token=fixture-secret" }, {
	run_id = "run-gemini",
	session_id = "gemini-fixture",
})[1]
assert(
	warning.type == "run.error" and warning.payload.message == "token=[REDACTED]" and warning.payload.retryable,
	"Gemini nonfatal stream errors must remain explicit and redacted"
)

local sensitive = stream.new()
local call = sensitive:feed({
	type = "tool_use",
	tool_id = "tool-fixture",
	tool_name = "read_file",
	parameters = { path = "README.md", token = "fixture-secret" },
}, { run_id = "run-gemini", session_id = "gemini-fixture" })[1]
assert(
	call.payload.input.path == "README.md" and call.payload.input.token == nil,
	"Gemini tool inputs must omit credentials before normalization"
)

local mismatch = stream.new()
assert(not pcall(mismatch.feed, mismatch, { type = "init", session_id = "other-session" }, {
	run_id = "run-gemini",
	session_id = "gemini-fixture",
}), "Gemini streams must reject cross-session native events")

local unavailable = stream.unavailable("token=fixture-secret")
assert(
	unavailable:status().state == "unavailable" and unavailable:status().reason == "token=[REDACTED]",
	"Gemini stream unavailability must remain explicit and redacted"
)
assert(not pcall(unavailable.feed, unavailable, { type = "init", session_id = "gemini-fixture" }, {
	run_id = "run-gemini",
}), "unavailable Gemini streams must reject records")

local cancelled = stream.new()
assert(cancelled:cancel("token=fixture-secret"), "ready Gemini streams must support cancellation")
assert(
	cancelled:status().state == "cancelled" and cancelled:status().reason == "token=[REDACTED]",
	"Gemini stream cancellation reasons must be redacted"
)
