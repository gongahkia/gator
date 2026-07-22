local core_lineage = require("gator").module("core").handoff_lineage
local evidence = require("gator").module("context").handoff_evidence
local guard = require("gator").module("context").handoff_guard
local pack = require("gator").module("context").handoff_pack

local source_pack = pack.new({ id = "pack-guard", task_id = "task-guard", entries = {} })
local function source(provider, id)
	return evidence.from_record({
		schema_version = 1,
		task_id = "task-guard",
		state = "ready",
		source = {
			provider = provider,
			run_id = "run-" .. provider,
			session = { provider = provider, id = id, owner = "provider" },
		},
		decisions = { { event_id = "decision-" .. provider, type = "message.thought", at = 1, summary = "reviewed" } },
		outcomes = {},
	})
end
local first = core_lineage.new({
	id = "lineage-guard",
	task_id = "task-guard",
	pack_id = "pack-guard",
	source = {
		provider = "codex",
		run_id = "run-codex",
		session = { provider = "codex", id = "native-codex", owner = "provider" },
	},
	target = { provider = "gemini", id = "native-gemini", owner = "provider" },
	snapshots = {},
	at = 1,
})
local value = guard.new({ lineages = { first } })
local duplicate =
	value:reserve({ pack = source_pack, evidence = source("codex", "native-codex"), target_provider = "gemini" })
assert(
	not duplicate.available and duplicate.reason:find("duplicate", 1, true),
	"recorded handoffs must block duplicate launches"
)
local cycle =
	value:reserve({ pack = source_pack, evidence = source("gemini", "native-gemini"), target_provider = "codex" })
assert(
	not cycle.available and cycle.reason:find("cycle", 1, true),
	"provider return paths must block accidental handoff cycles"
)

local fresh = guard.new({ lineages = {} })
local reserved =
	fresh:reserve({ pack = source_pack, evidence = source("codex", "native-new"), target_provider = "gemini" })
assert(
	guard.is_reservation(reserved) and reserved:status().state == "pending",
	"new handoffs must reserve one pending launch"
)
local pending =
	fresh:reserve({ pack = source_pack, evidence = source("codex", "native-new"), target_provider = "gemini" })
assert(
	not pending.available and pending.reason:find("duplicate", 1, true),
	"pending handoffs must block concurrent duplicates"
)
assert(reserved:cancel(), "cancelled reservations must release the handoff launch")
local retried =
	fresh:reserve({ pack = source_pack, evidence = source("codex", "native-new"), target_provider = "gemini" })
assert(guard.is_reservation(retried), "cancelled handoffs must permit a distinct retry reservation")
assert(
	retried:commit(core_lineage.new({
		id = "lineage-retry",
		task_id = "task-guard",
		pack_id = "pack-guard",
		source = {
			provider = "codex",
			run_id = "run-codex",
			session = { provider = "codex", id = "native-new", owner = "provider" },
		},
		target = { provider = "gemini", id = "native-new-target", owner = "provider" },
		snapshots = {},
		at = 2,
	})),
	"matching reservations must commit their completed lineage"
)
