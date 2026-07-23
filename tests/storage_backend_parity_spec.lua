local database = require("gator").module("core").database
local run = require("gator").module("core").run
local storage = require("gator").module("core").storage
local task = require("gator").module("core").task
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local root = helpers.tempdir("storage-backend-parity")
local json_path = root .. "/state.json"
local json = storage.resolve({ kind = "json", path = json_path }).backend
local sqlite = storage.resolve({ kind = "sqlite", path = root .. "/state.sqlite3" }).backend
assert(sqlite:migrate(root), "SQLite parity backend must initialize its schema")

local function exercise(backend)
	local first = task.new({
		id = "task-parity",
		objective = "backend token: private-value",
		created_at = 1,
		updated_at = 1,
	})
	backend:put_task(first)
	local value = run.new({
		id = "run-parity",
		task_id = "task-parity",
		provider = { name = "codex", session_id = "native-parity" },
		process = { pid = 1, executable = "codex" },
		workspace = { kind = "project", root = root },
		state = "completed",
		timing = { started_at = 2, ended_at = 3 },
		usage = {},
	})
	backend:append_run(value)
	backend:append_run_event("run-parity", {
		id = "event-parity",
		run_id = "run-parity",
		type = "stream.delta",
		at = 4,
		payload = { text = "event token: private-value" },
	})
	backend:append_evidence_excerpt({
		id = "evidence-parity",
		task_id = "task-parity",
		kind = "validation",
		at = 4,
		text = "evidence token: private-value",
	})
	local updated = task.new({
		id = "task-parity",
		objective = "completed backend parity",
		created_at = 1,
		updated_at = 5,
	})
	backend:commit_task_operation({ task = updated, operation = { id = "operation-parity", kind = "review", at = 5 } })
	local bundle = vim.json.decode(backend:export_bundle({ task_id = "task-parity" }))
	assert(
		bundle.tasks[1].objective == "completed backend parity"
			and bundle.runs[1].provider.session_id == "native-parity"
			and not bundle.runs[1].events[1].payload.text:find("private%-value")
			and not bundle.evidence_excerpts[1].text:find("private%-value")
			and bundle.operations[1].id == "operation-parity",
		"storage backends must preserve canonical records while redacting durable text"
	)
	assert(
		not pcall(backend.append_run, backend, value)
			and not pcall(backend.delete_evidence_excerpt, backend, "evidence-parity", false)
			and backend:delete_evidence_excerpt("evidence-parity", true).verified,
		"storage backends must reject duplicate writes and require confirmed evidence deletion"
	)
	return bundle
end

local json_bundle = exercise(json)
local sqlite_bundle = exercise(sqlite)
assert(
	vim.deep_equal(json_bundle, sqlite_bundle) and vim.fn.filereadable(json_path) == 1,
	"SQLite and real-file JSON backends must provide the same persisted export contract"
)
assert(
	database.open(root .. "/state.sqlite3"):version() == database.schema_version,
	"SQLite parity coverage must leave the production schema readable"
)
