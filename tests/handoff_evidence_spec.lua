local evidence = require("gator").module("context").handoff_evidence
local approval = require("gator").module("core").approval_event
local error_event = require("gator").module("core").error_event
local file_event = require("gator").module("core").file_event
local provider_event = require("gator").module("core").provider_event
local task = require("gator").module("core").task
local tool_event = require("gator").module("core").tool_event

local source_task = task.new({ id = "task-evidence", objective = "preserve source outcomes", lifecycle = "running" })
local function event(id, sequence, event_type, payload)
	return provider_event.new({
		schema_version = provider_event.schema_version,
		id = id,
		run_id = "run-source",
		provider = { name = "codex", session_id = "native-source" },
		sequence = sequence,
		at = sequence,
		type = event_type,
		payload = payload,
	})
end

local captured = evidence.capture({
	task = source_task,
	events = {
		event("message-completed", 5, "message.completed", { text = "implemented token=fixture-secret" }),
		tool_event.result({
			id = "tool-result",
			run_id = "run-source",
			provider = { name = "codex", session_id = "native-source" },
			sequence = 4,
			at = 4,
			call_id = "call-source",
			state = "completed",
			output = "token=fixture-secret",
		}),
		file_event.change({
			id = "file-change",
			run_id = "run-source",
			provider = { name = "codex", session_id = "native-source" },
			sequence = 3,
			at = 3,
			path = "lua/gator/context.lua",
			kind = "modified",
		}),
		approval.decision({
			id = "permission-decision",
			run_id = "run-source",
			provider = { name = "codex", session_id = "native-source" },
			sequence = 2,
			at = 2,
			request_id = "request-source",
			decision = "approved",
		}),
		event("message-thought", 1, "message.thought", { summary = "inspect token=fixture-secret" }),
		tool_event.call({
			id = "tool-call",
			run_id = "run-source",
			provider = { name = "codex", session_id = "native-source" },
			sequence = 3,
			at = 3,
			call_id = "call-source",
			name = "read_file",
		}),
		error_event.new({
			id = "run-error",
			run_id = "run-source",
			provider = { name = "codex", session_id = "native-source" },
			sequence = 6,
			at = 6,
			kind = "transport",
			message = "token=fixture-secret",
			retryable = true,
		}),
	},
})
assert(evidence.is(captured) and captured.state == "ready", "source evidence must create a ready immutable record")
assert(
	captured.source.provider == "codex"
		and captured.source.run_id == "run-source"
		and captured.source.session.id == "native-source"
		and captured.source.session.owner == "provider",
	"source evidence must preserve a provider-owned source session reference"
)
assert(
	vim.deep_equal(
		vim.tbl_map(function(value)
			return value.event_id
		end, captured.decisions),
		{ "message-thought", "permission-decision", "tool-call" }
	)
		and vim.deep_equal(
			vim.tbl_map(function(value)
				return value.event_id
			end, captured.outcomes),
			{ "file-change", "tool-result", "message-completed", "run-error" }
		),
	"source evidence must separate ordered decision and outcome events"
)
local rendered = vim.json.encode(evidence.to_record(captured))
assert(not rendered:find("fixture-secret", 1, true), "handoff evidence must redact source secrets and omit tool output")
local record = evidence.to_record(captured)
record.outcomes[1].summary = "mutated"
assert(captured.outcomes[1].summary ~= "mutated", "handoff evidence records must not mutate captured evidence")
assert(
	evidence.from_record(evidence.to_record(captured)).source.session.id == "native-source",
	"handoff evidence must round-trip canonical records"
)

local unavailable = evidence.capture({
	task = source_task,
	events = { event("message-delta", 1, "message.delta", { text = "partial" }) },
})
assert(
	unavailable.state == "unavailable" and unavailable.reason:find("no handoff decision or outcome evidence", 1, true),
	"source evidence must expose unavailable captures"
)
assert(not pcall(evidence.capture, {
	task = source_task,
	events = {
		event("source-one", 1, "message.thought", { summary = "one" }),
		provider_event.new({
			schema_version = provider_event.schema_version,
			id = "source-two",
			run_id = "run-other",
			provider = { name = "claude", session_id = "native-other" },
			sequence = 2,
			at = 2,
			type = "message.completed",
			payload = { text = "two" },
		}),
	},
}), "source evidence must reject mixed provider-native source runs")
assert(
	not pcall(evidence.capture, { task = source_task, events = { { type = "message.thought" } } }),
	"source evidence must require normalized provider events"
)
