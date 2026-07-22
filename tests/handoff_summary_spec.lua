local summary = require("gator").module("context").handoff_summary
local handoff_pack = require("gator").module("context").handoff_pack
local pack = require("gator").module("context").pack

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
