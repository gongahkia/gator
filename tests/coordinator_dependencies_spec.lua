local dependencies = require("gator.coordinator.dependencies")

local loads = 0
local value = dependencies.new({
	loader = function(path)
		loads = loads + 1
		assert(path == "gator.core", "container must resolve the registered module path")
		return { name = "core", api_version = 1 }
	end,
})

assert(dependencies.is(value), "dependency container must expose a typed interface")
assert(loads == 0, "dependency construction must not eagerly load services")
assert(value:module("core").name == "core" and loads == 1, "container must lazily load validated public modules")
assert(value:status("core").available and loads == 1, "container must cache successful dependency resolution")
assert(not pcall(value.module, value, "config"), "container must reject non-public module requests")
assert(not pcall(value.status, value, "missing"), "container must reject unknown dependencies")

local unavailable = dependencies
	.new({
		loader = function()
			error("token: private-secret")
		end,
	})
	:status("core")

assert(
	not unavailable.available and unavailable.reason:find("private%-secret") == nil,
	"container must expose dependency failures without leaking secrets"
)
assert(not pcall(dependencies.new, { loader = true }), "container must validate dependency loaders")
