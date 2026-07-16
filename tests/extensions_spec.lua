local extensions = require("gator").module("extensions").manager
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local palette = require("gator.ui.palette")

local directory = helpers.tempdir("extensions")
helpers.write(
	directory .. "/fixture.lua",
	[[
return {
  name = "fixture",
  api_version = 1,
  setup = function(api)
    api.palette.register({ kind = "action", name = "extension-action", execute = function() return "loaded" end })
    api.health.register("ready", function(report) report.ok("fixture ready") end)
    return function() end
  end,
}
]]
)
local manager = extensions.open({ paths = { directory } })
assert(
	vim.deep_equal(manager:discover(), { "fixture" }),
	"extensions must discover validated manifests deterministically"
)
assert(manager:load("fixture").name == "fixture", "extensions must load through the isolated public API")
assert(palette.execute("action:extension-action") == "loaded", "loaded extensions must register public palette actions")
assert(manager:list()[1].loaded, "extension state must report loaded manifests")
assert(manager:unload("fixture"), "extensions must unload registered resources")
assert(not palette.unregister("action:extension-action"), "extension unload must remove palette registrations")
assert(not manager:unload("fixture"), "unloading an absent extension must be explicit and idempotent")
helpers.write(directory .. "/invalid.lua", "return { name = 'invalid', api_version = 2, setup = function() end }")
assert(not pcall(manager.discover, manager), "unsupported extension APIs must fail explicitly")
