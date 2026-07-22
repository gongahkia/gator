local handoff_pack = require("gator").module("context").handoff_pack

local entries = {
	{
		id = "entry-task",
		kind = "task",
		ref = "task-handoff",
		provenance = { source = "task", ref = "task-handoff" },
		trust = "repository",
		token_estimate = { status = "estimated", tokens = 10 },
		transfer = { eligible = true },
	},
	{
		id = "entry-instruction",
		kind = "instruction",
		ref = "instruction:1",
		provenance = { source = "user", ref = "prompt:1" },
		trust = "manual",
		token_estimate = { status = "unavailable", reason = "not measured" },
		transfer = { eligible = false, reason = "requires review" },
	},
}

local value = handoff_pack.new({ id = "handoff-pack", task_id = "task-handoff", entries = entries })
assert(handoff_pack.is(value), "handoff packs must have a distinct immutable value type")
assert(
	value.id == "handoff-pack" and value.task_id == "task-handoff" and value.entries[1].id == "entry-task",
	"handoff packs must preserve their ordered curated entries"
)
entries[1].ref = "mutated-input"
assert(value.entries[1].ref == "task-handoff", "handoff packs must not retain caller-owned entries")

local record = handoff_pack.to_record(value)
record.entries[1].ref = "mutated-record"
assert(value.entries[1].ref == "task-handoff", "handoff records must not mutate immutable packs")
assert(
	handoff_pack.from_record(handoff_pack.to_record(value)).entries[2].id == "entry-instruction",
	"handoff packs must round-trip canonical records"
)
assert(
	not pcall(handoff_pack.new, { id = "handoff-pack", task_id = "task-handoff", entries = {}, extra = true }),
	"handoff packs must reject unsupported fields"
)
assert(
	not pcall(handoff_pack.new, { id = "handoff-pack", task_id = "task-handoff", entries = { { id = "invalid" } } }),
	"handoff packs must reject invalid context entries"
)
assert(
	not pcall(
		handoff_pack.from_record,
		{ schema_version = 2, id = "handoff-pack", task_id = "task-handoff", entries = {} }
	),
	"handoff packs must reject unsupported record schemas"
)
