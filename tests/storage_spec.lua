local storage = require("gator").module("core").storage
local database = require("gator").module("core").database
local run = require("gator").module("core").run
local task = require("gator").module("core").task
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local backend = { api_version = 1 }
for _, name in ipairs(storage.json_contract().methods) do
	backend[name] = function() end
end
assert(storage.validate_json_backend(backend) == backend, "JSON storage contracts must expose every versioned method")
backend.append_run = nil
assert(not pcall(storage.validate_json_backend, backend), "JSON storage contracts must reject incomplete backends")

local filesystem = require("gator").module("core").filesystem
local files = {}
local selected = storage.resolve({
	kind = "json",
	path = "/fixture/state.json",
	filesystem = filesystem.new({
		readable = function(path)
			return files[path] ~= nil
		end,
		read = function(path)
			return files[path]
		end,
		mkdir = function()
			return true
		end,
		write = function(path, value)
			files[path] = value
			return true
		end,
		rename = function(source, target)
			files[target], files[source] = files[source], nil
			return true
		end,
		remove = function(path)
			files[path] = nil
			return true
		end,
	}),
})
assert(
	selected.kind == "json" and selected.backend.api_version == 1,
	"storage resolution must select the injected JSON backend"
)
assert(not pcall(storage.resolve, { kind = "missing" }), "storage resolution must reject unavailable backends")

local root = helpers.tempdir("storage-migration")
local source = database.open(root .. "/state.sqlite3")
assert(source:migrate(root), "SQLite source must migrate before export")
source:put_task(task.new({ id = "task-migrate", objective = "Migrate storage", created_at = 1, updated_at = 1 }))
local migrated = storage.migrate_sqlite_to_json({ source = source, target = selected.backend, confirm = true })
assert(
	migrated.tasks[1].id == "task-migrate" and selected.backend:get_task("task-migrate").objective == "Migrate storage",
	"confirmed migrations must preserve SQLite task records in JSON"
)
assert(
	not pcall(storage.migrate_sqlite_to_json, { source = source, target = selected.backend, confirm = true }),
	"migrations must reject non-empty JSON targets"
)
assert(
	not pcall(storage.migrate_sqlite_to_json, { source = source, target = selected.backend, confirm = false }),
	"migrations must require explicit confirmation"
)

local json_source = storage.resolve({
	kind = "json",
	path = "/fixture/json-to-sqlite.json",
	filesystem = selected.backend.filesystem,
}).backend
local json_task =
	task.new({ id = "task-json-migrate", objective = "Restore JSON storage", created_at = 2, updated_at = 2 })
json_source:put_task(json_task)
json_source:append_run(run.new({
	id = "run-json-migrate",
	task_id = "task-json-migrate",
	provider = { name = "codex", session_id = "native-migrate" },
	process = { pid = 2, executable = "codex" },
	workspace = { kind = "project", root = root },
	state = "completed",
	timing = { started_at = 2, ended_at = 3 },
	usage = {},
	events = { { id = "event-json-migrate", run_id = "run-json-migrate", type = "stream.delta", at = 3, payload = {} } },
}))
json_source:append_evidence_excerpt({
	id = "evidence-json-migrate",
	task_id = "task-json-migrate",
	kind = "summary",
	at = 3,
	text = "Migrated evidence",
})
json_source:commit_task_operation({
	task = json_task,
	operation = { id = "operation-json-migrate", kind = "review", at = 3 },
})
local sqlite_target = database.open(root .. "/json-to-sqlite.sqlite3")
local imported = storage.migrate_json_to_sqlite({ source = json_source, target = sqlite_target, confirm = true })
assert(
	imported.tasks[1].id == "task-json-migrate"
		and sqlite_target:get_run("run-json-migrate").events[1].id == "event-json-migrate"
		and sqlite_target:list_evidence_excerpts("task-json-migrate")[1].id == "evidence-json-migrate"
		and sqlite_target:list_task_operations("task-json-migrate")[1].id == "operation-json-migrate",
	"confirmed migrations must preserve JSON tasks, runs, evidence, and operations in SQLite"
)
assert(
	not pcall(storage.migrate_json_to_sqlite, { source = json_source, target = sqlite_target, confirm = true }),
	"migrations must reject non-empty SQLite targets"
)
local cancelled = database.open(root .. "/cancelled.sqlite3")
assert(
	not pcall(storage.migrate_json_to_sqlite, { source = json_source, target = cancelled, confirm = false })
		and cancelled:version() == 0,
	"JSON-to-SQLite migrations must cancel before initializing the target"
)
