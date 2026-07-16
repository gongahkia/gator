local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local config = require("gator.config")

local path = helpers.tempdir("config") .. "/gator.json"
helpers.write(path, '{"ui":{"layout":"modal"},"workspaces":{"mode":"worktree","max_write_runs":2}}')
local value = config.load(path)
assert(
	value.ui.layout == "modal" and value.workspaces.mode == "worktree" and value.workspaces.max_write_runs == 2,
	"global settings must load user defaults"
)
assert(
	not pcall(config.resolve, { telemetry = { token = "secret" } }),
	"global settings must reject provider credentials"
)
helpers.write(path, "not-json")
assert(not pcall(config.load, path), "invalid user defaults must fail explicitly")
