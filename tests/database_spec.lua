local database = require("gator").module("core").database
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local root = helpers.tempdir("database")
local db = database.open(root .. "/state.sqlite3")

helpers.write(
	root .. "/sessions.json",
	vim.json.encode({
		schema_version = 1,
		sessions = { { task_id = "task-one", provider = "codex", id = "native-one" } },
	})
)
helpers.write(
	root .. "/threads.json",
	vim.json.encode({
		schema_version = 1,
		threads = { { id = "thread-one", task_id = "task-one", entries = {} } },
	})
)
helpers.write(
	root .. "/runs.json",
	vim.json.encode({
		schema_version = 1,
		runs = { { id = "run-one", task_id = "task-one", events = {} } },
	})
)

assert(db:migrate(root), "new local state must migrate")
assert(db:version() == 1, "SQLite migrations must record schema version")
assert(db:exec("SELECT count(*) FROM session_metadata;"):match("1"), "active session metadata must survive migration")
assert(db:exec("SELECT count(*) FROM threads;"):match("1"), "thread evidence must survive migration")
assert(db:exec("SELECT count(*) FROM runs;"):match("1"), "run evidence must survive migration")
assert(not db:migrate(root), "current schemas must not reapply migrations")
assert(vim.fn.filereadable(root .. "/sessions.json") == 1, "migration must retain legacy state files")

local corrupt = helpers.tempdir("corrupt-database")
helpers.write(corrupt .. "/sessions.json", "not json")
local ok = pcall(function()
	database.open(corrupt .. "/state.sqlite3"):migrate(corrupt)
end)
assert(not ok, "invalid local state must fail before destructive migration")
