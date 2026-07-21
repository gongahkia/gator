local database = require("gator").module("core").database
local task = require("gator").module("core").task
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
assert(db:version() == 2, "SQLite migrations must record every schema version")
assert(db:exec("SELECT count(*) FROM session_metadata;"):match("1"), "active session metadata must survive migration")
assert(db:exec("SELECT count(*) FROM threads;"):match("1"), "thread evidence must survive migration")
assert(db:exec("SELECT count(*) FROM runs;"):match("1"), "run evidence must survive migration")
assert(not db:migrate(root), "current schemas must not reapply migrations")
assert(vim.fn.filereadable(root .. "/sessions.json") == 1, "migration must retain legacy state files")

local first = task.new({
	id = "task-one",
	objective = "Persist canonical task records",
	lifecycle = "planned",
	sessions = { { provider = "codex", id = "native-one", owner = "provider" } },
	created_at = 1,
	updated_at = 2,
})
assert(
	task.is(db:put_task(first))
		and db:get_task("task-one").sessions[1].owner == "provider"
		and db:get_task("task-one").objective == first.objective,
	"SQLite task storage must preserve validated canonical provider-owned records"
)
local second = task.new({ id = "task-two", objective = "Second task", created_at = 3, updated_at = 3 })
db:put_task(second)
assert(
	vim.deep_equal(
		vim.tbl_map(function(value)
			return value.id
		end, db:list_tasks()),
		{ "task-one", "task-two" }
	),
	"SQLite task storage must list canonical records in deterministic order"
)
assert(not db:get_task("task-missing"), "SQLite task storage must expose missing records")

local pending = database.open(helpers.tempdir("pending-database") .. "/state.sqlite3")
assert(not pcall(pending.put_task, pending, first), "SQLite task writes must require completed migrations")

local previous = helpers.tempdir("v1-database") .. "/state.sqlite3"
local upgraded = database.open(previous)
upgraded:exec("PRAGMA user_version = 1;")
assert(upgraded:migrate() and upgraded:version() == 2, "SQLite migrations must upgrade version one databases")

local corrupt = helpers.tempdir("corrupt-database")
helpers.write(corrupt .. "/sessions.json", "not json")
local ok = pcall(function()
	database.open(corrupt .. "/state.sqlite3"):migrate(corrupt)
end)
assert(not ok, "invalid local state must fail before destructive migration")
