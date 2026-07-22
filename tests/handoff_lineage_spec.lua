local database = require("gator").module("core").database
local json_backend = require("gator").module("core").json_backend
local session = require("gator").module("core").session
local storage = require("gator").module("core").storage
local task = require("gator").module("core").task
local evidence = require("gator").module("context").handoff_evidence
local lineage = require("gator").module("context").handoff_lineage
local pack = require("gator").module("context").handoff_pack
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local entity = task.new({ id = "task-lineage", objective = "preserve handoff lineage", created_at = 1, updated_at = 1 })
local source_pack = pack.new({ id = "pack-lineage", task_id = entity.id, entries = {} })
local source_evidence = evidence.from_record({
	schema_version = 1,
	task_id = entity.id,
	state = "ready",
	source = {
		provider = "codex",
		run_id = "run-source",
		session = { provider = "codex", id = "native-sk-source", owner = "provider" },
	},
	decisions = { { event_id = "decision-source", type = "message.thought", at = 1, summary = "reviewed" } },
	outcomes = {},
})
local target = session.new({
	task_id = entity.id,
	provider = "gemini",
	id = "native-sk-target",
	owner = "provider",
})
local value = lineage.new({
	id = "lineage-one",
	pack = source_pack,
	evidence = source_evidence,
	target = target,
	source_snapshot = {
		id = "snapshot-source",
		run_id = "run-source",
		type = "workspace.git_snapshot",
		at = 2,
		payload = { root = "/tmp/token=fixture-secret" },
	},
	target_snapshot = { id = "snapshot-target", run_id = "run-target", type = "workspace.git_snapshot", at = 3 },
	at = 4,
})
local rendered = vim.inspect(lineage.to_record(value))
assert(
	lineage.is(value)
		and value.source.session.id == "native-sk-source"
		and value.target.id == "native-sk-target"
		and value.snapshots.source.payload == nil
		and not rendered:find("fixture-secret", 1, true),
	"lineage records must preserve provider-owned source and target sessions with path-free snapshot references"
)

local root = helpers.tempdir("handoff-lineage")
local json = json_backend.open(root .. "/state.json")
json:put_task(entity)
assert(
	lineage.persist({ lineage = value, backend = json }).id == "lineage-one"
		and json:get_handoff_lineage("lineage-one").source.session.id == "native-sk-source"
		and json:list_handoff_lineages(entity.id)[1].target.provider == "gemini",
	"JSON storage must append and recover exact handoff lineage references"
)
assert(
	not pcall(lineage.persist, { lineage = value, backend = json }),
	"handoff lineage storage must reject duplicate append-only records"
)

local sqlite = database.open(root .. "/state.sqlite3")
assert(sqlite:migrate(root), "SQLite storage must migrate handoff lineage tables")
sqlite:put_task(entity)
assert(
	lineage.persist({ lineage = value, backend = sqlite }).snapshots.target.id == "snapshot-target"
		and sqlite:get_handoff_lineage("lineage-one").target.id == "native-sk-target"
		and #sqlite:preview_export({ task_id = entity.id }).handoff_lineages == 1,
	"SQLite storage must persist, export, and recover lineage references"
)
local replica = json_backend.open(root .. "/replica.json")
storage.migrate_sqlite_to_json({ source = sqlite, target = replica, confirm = true })
local restored = database.open(root .. "/restored.sqlite3")
storage.migrate_json_to_sqlite({ source = replica, target = restored, confirm = true })
assert(
	replica:get_handoff_lineage("lineage-one").source.session.id == "native-sk-source"
		and restored:get_handoff_lineage("lineage-one").snapshots.source.id == "snapshot-source",
	"storage migrations must preserve provider sessions and snapshot references"
)

local unavailable = lineage.new({
	id = "lineage-unavailable",
	pack = source_pack,
	evidence = evidence.from_record({
		schema_version = 1,
		task_id = entity.id,
		state = "unavailable",
		reason = "source handoff evidence is unavailable",
	}),
	target = target,
	at = 5,
})
assert(
	not unavailable.available and unavailable.reason:find("unavailable", 1, true),
	"lineage creation must explicitly remain unavailable without source evidence"
)
assert(
	not pcall(
		lineage.new,
		{ id = "lineage-invalid", pack = source_pack, evidence = source_evidence, target = {}, at = 5 }
	),
	"lineage creation must reject missing provider-owned target sessions"
)
