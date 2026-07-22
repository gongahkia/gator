local handoff_pack = require("gator").module("context").handoff_pack
local context_pack = require("gator").module("context").pack
local task = require("gator").module("core").task

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

local function entry(id, kind)
	return context_pack.entry({
		id = id,
		kind = kind,
		ref = kind .. "://" .. id,
		provenance = { source = "fixture", ref = id },
		trust = "manual",
		token_estimate = { status = "estimated", tokens = 1 },
		transfer = { eligible = true },
	})
end

local source_task = task.new({
	id = "task-build-handoff",
	objective = "continue with token=fixture-secret",
	lifecycle = "planned",
	sessions = { { provider = "codex", id = "native-session", owner = "provider" } },
})
local built = handoff_pack.build({
	id = "handoff-build",
	task = source_task,
	files = { { entry = entry("file-handoff", "file") } },
	diffs = { { entry = entry("diff-handoff", "diff") } },
	diagnostics = { { entry = entry("diagnostic-handoff", "diagnostic") } },
	instructions = { entry("instruction-handoff", "instruction") },
})
assert(
	built.task_id == source_task.id
		and vim.deep_equal(
			vim.tbl_map(function(value)
				return value.id
			end, built.entries),
			{ "task-task-build-handoff", "file-handoff", "diff-handoff", "diagnostic-handoff", "instruction-handoff" }
		),
	"handoff packs must order task, file, diff, diagnostic, and instruction sources"
)
assert(
	built.entries[1].content:find("fixture-secret", 1, true) == nil
		and built.entries[1].content ~= ""
		and built.entries[1].ref == "gator-task://task-build-handoff",
	"task handoff entries must contain redacted task intent without provider session ownership"
)
assert(
	not pcall(handoff_pack.build, { id = "handoff-invalid", task = source_task, files = "invalid" }),
	"handoff pack building must reject invalid source groups"
)
assert(
	not pcall(handoff_pack.build, { id = "handoff-invalid", task = {}, files = {} }),
	"handoff pack building must require canonical task sources"
)

local evaluated_source = handoff_pack.new({
	id = "pack-handoff-trust",
	task_id = "task-handoff",
	entries = {
		vim.tbl_extend("force", entry("entry-provenance-handoff", "file"), { trust = "provenance" }),
		vim.tbl_extend("force", entry("entry-repository-handoff", "diff"), { trust = "repository" }),
		vim.tbl_extend("force", entry("entry-manual-handoff", "instruction"), { trust = "manual" }),
	},
})
local evaluated, decisions = handoff_pack.evaluate({ pack = evaluated_source, mode = "repository" })
assert(
	#decisions == #evaluated_source.entries
		and decisions[1].allowed
		and decisions[2].allowed
		and not decisions[3].allowed
		and decisions[2].provenance.ref == "entry-repository-handoff"
		and decisions[3].trust == "manual",
	"handoff trust evaluation must audit provenance and trust for every ordered entry"
)
assert(
	evaluated.entries[2].policy_decision == "allowed by repository trust"
		and evaluated.entries[3].policy_decision:find("does not allow manual", 1, true)
		and evaluated_source.entries[2].policy_decision == nil,
	"handoff trust evaluation must visibly annotate a new immutable pack without mutating its source"
)
local strict, strict_decisions = handoff_pack.evaluate({ pack = evaluated_source, mode = "manual" })
assert(
	#strict.entries == 3
		and not strict_decisions[1].allowed
		and strict.entries[1].policy_decision:find("strict manual", 1, true),
	"manual handoff trust must deny every entry pending explicit selection"
)
assert(
	not pcall(handoff_pack.evaluate, { pack = evaluated_source, mode = "invalid" })
		and not pcall(handoff_pack.evaluate, { pack = {}, mode = "manual" }),
	"handoff trust evaluation must reject invalid policies and pack records"
)
