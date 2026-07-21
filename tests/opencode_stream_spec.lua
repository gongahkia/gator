local fixtures = require("gator").module("adapters").fixtures
local stream = require("gator").module("adapters").opencode_stream
local value = stream.new({
	event_id = function(context)
		return "opencode-event-" .. context.sequence
	end,
	now = function()
		return 7
	end,
})
local events = {}
fixtures.replay_jsonrpc(vim.g.gator_test.root .. "/tests/fixtures/adapters/opencode_stream.jsonl", function(record)
	for _, event in ipairs(value:feed(record, { run_id = "run-opencode", session_id = "opencode-fixture" })) do
		table.insert(events, event)
	end
end)
assert(
	#events == 7
		and events[1].type == "message.delta"
		and events[1].payload.text == "gator-fixture"
		and events[2].type == "message.thought"
		and events[3].type == "tool.call"
		and events[3].payload.input.token == nil
		and events[4].type == "tool.result"
		and events[4].payload.state == "completed"
		and events[4].payload.output.token == nil
		and events[5].type == "file.change"
		and events[5].payload.path == "lua/gator/init.lua"
		and events[6].type == "usage.update"
		and events[6].payload.used == 3
		and events[7].type == "run.error"
		and events[7].payload.message == "token=[REDACTED]",
	"OpenCode streams must normalize ACP updates without retaining credentials"
)
assert(
	stream.signals().usage.available
		and stream.signals().file_changes.available
		and not stream.signals().compaction.available,
	"OpenCode signal support must expose unavailable compaction explicitly"
)

local unsafe = stream.new()
unsafe:feed({
	jsonrpc = "2.0",
	method = "session/update",
	params = {
		sessionId = "opencode-fixture",
		update = {
			sessionUpdate = "tool_call",
			toolCallId = "unsafe-tool",
			title = "write",
			kind = "edit",
			status = "pending",
			rawInput = { filePath = "/outside/repository" },
		},
	},
}, { run_id = "run-opencode", session_id = "opencode-fixture" })
local unsafe_events = unsafe:feed({
	jsonrpc = "2.0",
	method = "session/update",
	params = {
		sessionId = "opencode-fixture",
		update = { sessionUpdate = "tool_call_update", toolCallId = "unsafe-tool", status = "completed" },
	},
}, { run_id = "run-opencode", session_id = "opencode-fixture" })
assert(
	#unsafe_events == 1 and unsafe_events[1].type == "tool.result",
	"OpenCode file-change signals must omit unsafe paths"
)

local mismatch = stream.new()
assert(not pcall(mismatch.feed, mismatch, {
	jsonrpc = "2.0",
	method = "session/update",
	params = { sessionId = "other-session", update = { sessionUpdate = "available_commands_update" } },
}, { run_id = "run-opencode", session_id = "opencode-fixture" }), "OpenCode streams must reject cross-session updates")

local unavailable = stream.unavailable("token=fixture-secret")
assert(
	unavailable:status().state == "unavailable" and unavailable:status().reason == "token=[REDACTED]",
	"OpenCode stream unavailability must remain explicit and redacted"
)

local cancelled = stream.new()
assert(cancelled:cancel("token=fixture-secret"), "ready OpenCode streams must support cancellation")
assert(
	cancelled:status().state == "cancelled" and cancelled:status().reason == "token=[REDACTED]",
	"OpenCode stream cancellation reasons must be redacted"
)
