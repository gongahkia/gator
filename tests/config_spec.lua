local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local config = require("gator.config")

assert(config.resolve().schema_version == config.schema_version, "default settings must resolve to the current schema")

local path = helpers.tempdir("config") .. "/gator.json"
helpers.write(path, '{"schema_version":2,"ui":{"layout":"modal"},"workspaces":{"mode":"worktree","max_write_runs":2}}')
local value = config.load(path)
assert(
	value.schema_version == 2
		and value.ui.layout == "modal"
		and value.workspaces.mode == "worktree"
		and value.workspaces.max_write_runs == 2,
	"global settings must load user defaults"
)
assert(
	not pcall(config.resolve, { telemetry = { token = "secret" } }),
	"global settings must reject provider credentials"
)
helpers.write(path, "not-json")
assert(not pcall(config.load, path), "invalid user defaults must fail explicitly")
assert(not pcall(config.resolve, { schema_version = 1 }), "legacy configuration schemas must fail explicitly")
assert(not pcall(config.resolve, { schema_version = "2" }), "configuration schemas must require an integer version")

helpers.write(path, '{"schema_version":1,"ui":{"layout":"modal"}}')
local migrated, migrated_provenance, migrations = config.load(path)
assert(
	migrated.schema_version == 2
		and migrated.ui.layout == "modal"
		and migrations[1].from_version == 1
		and migrations[1].to_version == 2
		and migrated_provenance.schema_version.source == "migration",
	"legacy configuration files must migrate to schema v2 with provenance"
)
local legacy = '{"context":{"mode":"inspect"}}'
helpers.write(path, legacy)
assert(
	config.load(path).schema_version == 2
		and config.load(path).context.mode == "inspect"
		and table.concat(vim.fn.readfile(path), "\n") == legacy,
	"unversioned legacy configuration files must migrate without a durable rewrite"
)
helpers.write(path, '{"schema_version":3}')
assert(not pcall(config.load, path), "unknown file schemas must fail before migration")

local layered = config.resolve_sources({
	{
		source = "setup",
		ref = "gator.setup",
		settings = { ui = { layout = "modal" }, workspaces = { max_write_runs = 3 } },
	},
	{
		source = "file",
		ref = path,
		settings = { ui = { layout = "adaptive", screen_reader = false }, workspaces = { max_write_runs = 2 } },
	},
})
assert(
	layered.settings.ui.layout == "modal"
		and not layered.settings.ui.screen_reader
		and layered.settings.workspaces.max_write_runs == 3
		and layered.provenance["ui.layout"].source == "setup"
		and layered.provenance["ui.screen_reader"].source == "file"
		and layered.provenance["context.mode"].source == "defaults",
	"configuration layers must use deterministic precedence with field provenance"
)
assert(not pcall(config.resolve_sources, {
	{ source = "file", ref = path, settings = {} },
	{ source = "file", ref = path .. ".override", settings = {} },
}), "configuration layers must reject duplicate sources")
