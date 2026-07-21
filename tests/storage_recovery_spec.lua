local database = require("gator").module("core").database
local filesystem = require("gator").module("core").filesystem
local json_backend = require("gator").module("core").json_backend
local storage = require("gator").module("core").storage
local task = require("gator").module("core").task
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local rollback_root = helpers.tempdir("storage-rollback")
local duplicate = { task_id = "task-rollback", provider = "codex", id = "native-rollback" }
helpers.write(
	rollback_root .. "/sessions.json",
	vim.json.encode({ schema_version = 1, sessions = { duplicate, vim.deepcopy(duplicate) } })
)
local rollback = database.open(rollback_root .. "/state.sqlite3")
assert(
	not pcall(rollback.migrate, rollback, rollback_root)
		and rollback:version() == 0
		and vim.trim(rollback:exec("SELECT count(*) FROM sqlite_master WHERE type = 'table';")) == "0",
	"failed SQLite migrations must roll back schema and data changes"
)
helpers.write(rollback_root .. "/sessions.json", vim.json.encode({ schema_version = 1, sessions = { duplicate } }))
assert(
	rollback:migrate(rollback_root)
		and rollback:version() == database.schema_version
		and vim.trim(rollback:exec("SELECT count(*) FROM session_metadata;")) == "1",
	"SQLite migrations must recover after corrupt legacy input is corrected"
)

local files = { ["/fixture/corrupt.json"] = "not JSON" }
local fs = filesystem.new({
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
})
local corrupt = json_backend.open("/fixture/corrupt.json", { filesystem = fs })
assert(
	not pcall(corrupt.list_tasks, corrupt) and files["/fixture/corrupt.json"] == "not JSON",
	"corrupt JSON task storage must fail without overwriting evidence"
)
files["/fixture/corrupt.json"] =
	vim.json.encode({ schema_version = 1, tasks = {}, runs = {}, evidence_excerpts = {}, operations = {} })
corrupt:put_task(
	task.new({ id = "task-recovered", objective = "Recover JSON storage", created_at = 1, updated_at = 1 })
)
assert(
	corrupt:get_task("task-recovered").id == "task-recovered",
	"corrected JSON storage must recover without migration state"
)

local source_root = helpers.tempdir("storage-transfer")
local source = database.open(source_root .. "/state.sqlite3")
assert(source:migrate(source_root), "SQLite transfer source must migrate")
source:put_task(
	task.new({ id = "task-transfer", objective = "Preserve source on target failure", created_at = 1, updated_at = 1 })
)
local target_files = {}
local failing_target = json_backend.open("/fixture/transfer.json", {
	filesystem = filesystem.new({
		readable = function(path)
			return target_files[path] ~= nil
		end,
		read = function(path)
			return target_files[path]
		end,
		mkdir = function()
			return true
		end,
		write = function()
			return false
		end,
		rename = function()
			return true
		end,
		remove = function(path)
			target_files[path] = nil
			return true
		end,
	}),
})
assert(
	not pcall(storage.migrate_sqlite_to_json, { source = source, target = failing_target, confirm = true })
		and source:get_task("task-transfer").id == "task-transfer"
		and #failing_target:preview_export().tasks == 0,
	"failed SQLite-to-JSON transfers must retain source records and an empty target"
)

files["/fixture/corrupt-import.json"] = "not JSON"
local bad_source = json_backend.open("/fixture/corrupt-import.json", { filesystem = fs })
local untouched = database.open(helpers.tempdir("storage-corrupt-import") .. "/state.sqlite3")
assert(
	not pcall(storage.migrate_json_to_sqlite, { source = bad_source, target = untouched, confirm = true })
		and untouched:version() == 0,
	"corrupt JSON imports must fail before initializing SQLite targets"
)
