local summary = require("gator").module("context").handoff_summary
local evidence = require("gator").module("context").handoff_evidence
local handoff_pack = require("gator").module("context").handoff_pack
local pack = require("gator").module("context").pack

assert(
	summary.configure({ author = "gator", max_chars = 64 }).author == "gator" and summary.settings().max_chars == 64,
	"summary authoring settings must configure a validated default author and bound"
)
assert(
	not pcall(summary.configure, { author = "invalid", max_chars = 1 })
		and not pcall(summary.configure, { author = "user", max_chars = 0 }),
	"summary authoring settings must reject invalid values"
)
summary.configure(summary.defaults)

local source = handoff_pack.new({
	id = "pack-user-summary",
	task_id = "task-user-summary",
	entries = {
		pack.entry({
			id = "entry-user-summary",
			kind = "file",
			ref = "file://README.md",
			provenance = { source = "fixture", ref = "README.md" },
			trust = "manual",
			token_estimate = { status = "estimated", tokens = 1 },
			transfer = { eligible = true },
			content = "token=pack-secret",
		}),
	},
})

local value = summary.user({
	id = "summary-user",
	pack = source,
	content = "continue with token=summary-secret",
})
assert(summary.is(value), "user-authored handoff summaries must have a distinct immutable value type")
assert(
	value.author == "user"
		and value.pack_id == "pack-user-summary"
		and value.task_id == "task-user-summary"
		and value.content:find("summary-secret", 1, true) == nil,
	"user summaries must bind redacted text to the curated handoff pack"
)
local record = summary.to_record(value)
assert(
	vim.json.encode(record):find("pack-secret", 1, true) == nil,
	"user summary records must not retain raw handoff-pack content"
)
record.content = "mutated"
assert(value.content ~= "mutated", "user summary records must not mutate the immutable summary")
assert(
	summary.from_record(summary.to_record(value)).author == "user",
	"user summaries must round-trip canonical records"
)
assert(
	not pcall(summary.user, { id = "summary-invalid", pack = {}, content = "summary" }),
	"user summaries must require canonical handoff packs"
)
assert(not pcall(summary.from_record, {
	schema_version = 2,
	id = "summary-invalid",
	pack_id = "pack-user-summary",
	task_id = "task-user-summary",
	author = "user",
	content = "summary",
}), "user summaries must reject unsupported record schemas")

local source_evidence = evidence.new({
	task_id = "task-user-summary",
	state = "ready",
	source = {
		provider = "codex",
		run_id = "run-summary",
		session = { provider = "codex", id = "native-summary", owner = "provider" },
	},
	decisions = {
		{ event_id = "decision-summary", type = "message.thought", at = 1, summary = "inspect token=evidence-secret" },
	},
	outcomes = { { event_id = "outcome-summary", type = "message.completed", at = 2, summary = "implemented" } },
})
local generated = summary.synthesize({ id = "summary-gator", pack = source, evidence = source_evidence })
assert(
	generated.author == "gator"
		and generated.state == "ready"
		and generated.content:find("Source provider: codex", 1, true)
		and generated.content:find("Decisions:", 1, true)
		and generated.content:find("Outcomes:", 1, true),
	"local synthesis must create a deterministic summary from source evidence"
)
assert(
	generated.content:find("pack-secret", 1, true) == nil
		and generated.content:find("evidence-secret", 1, true) == nil
		and generated.content:find("native-summary", 1, true) == nil,
	"local synthesis must redact evidence and exclude raw pack content and source session IDs"
)
local unavailable = summary.synthesize({
	id = "summary-unavailable",
	pack = source,
	evidence = evidence.new({
		task_id = "task-user-summary",
		state = "unavailable",
		reason = "source evidence unavailable",
	}),
})
assert(
	unavailable.state == "unavailable"
		and unavailable.content == nil
		and unavailable.reason == "source evidence unavailable",
	"local synthesis must expose unavailable source evidence"
)
