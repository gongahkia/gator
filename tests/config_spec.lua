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
