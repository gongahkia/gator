local config = require("gator.config")
local database = require("gator").module("core").database
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local fixture_root = vim.g.gator_test.root .. "/tests/fixtures/migrations"
assert(vim.fn.isdirectory(fixture_root) == 1, "migration fixture corpus must exist")

for _, case in ipairs({
	{ name = "config-v1", layout = "modal" },
	{ name = "config-unversioned", mode = "inspect" },
}) do
	local path = helpers.tempdir("migration-" .. case.name) .. "/gator.json"
	local source = helpers.read(fixture_root .. "/" .. case.name .. ".json")
	helpers.write(path, source)
	local settings, provenance, migrations = config.load(path)
	assert(
		settings.schema_version == config.schema_version
			and migrations[1].from_version == 1
			and migrations[1].to_version == config.schema_version
			and provenance.schema_version.source == "migration"
			and (not case.layout or settings.ui.layout == case.layout)
			and (not case.mode or settings.context.mode == case.mode)
			and helpers.read(path) == source,
		"configuration migration corpus must upgrade " .. case.name .. " in memory without rewriting source"
	)
end
local unsupported = fixture_root .. "/config-unsupported.json"
assert(not pcall(config.load, unsupported), "configuration migration corpus must reject future schemas")

local root = helpers.tempdir("migration-legacy-state")
for _, name in ipairs({ "sessions", "threads", "runs" }) do
	helpers.write(root .. "/" .. name .. ".json", helpers.read(fixture_root .. "/" .. name .. "-v1.json"))
end
local db = database.open(root .. "/state.sqlite3")
assert(
	db:migrate(root)
		and db:version() == database.schema_version
		and db:exec("SELECT count(*) FROM session_metadata;"):match("1")
		and db:exec("SELECT count(*) FROM threads;"):match("1")
		and db:get_run("run-corpus").events[1].id == "event-corpus",
	"legacy-state migration corpus must preserve provider-native sessions, thread records, and run events"
)
for _, name in ipairs({ "sessions", "threads", "runs" }) do
	assert(
		helpers.read(root .. "/" .. name .. ".json") == helpers.read(fixture_root .. "/" .. name .. "-v1.json"),
		"legacy migration corpus must retain source " .. name .. " state"
	)
end

local malformed_root = helpers.tempdir("migration-malformed-state")
helpers.write(malformed_root .. "/sessions.json", helpers.read(fixture_root .. "/sessions-malformed.json"))
local rejected = database.open(malformed_root .. "/state.sqlite3")
assert(
	not pcall(rejected.migrate, rejected, malformed_root) and rejected:version() == 0,
	"malformed migration corpus input must fail before advancing the SQLite schema"
)
