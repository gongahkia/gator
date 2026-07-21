local diagnostics = require("gator").module("core").runtime_diagnostics
local runtime = require("gator").module("core").runtime

local owner = runtime.new({
	clock = function()
		return 1
	end,
})
owner:register("runtime-service", { start = function() end, stop = function() end })
owner:start("runtime-service")
local ready = diagnostics.capture({
	runtime = owner,
	diagnostics = {
		{
			name = "transport",
			check = function()
				return { state = "ready" }
			end,
		},
	},
})
assert(
	ready.state == "ready" and ready.services[1].state == "running" and ready.diagnostics[1].name == "transport",
	"runtime snapshots must expose running services and diagnostic health"
)

local degraded = diagnostics.capture({
	runtime = owner,
	diagnostics = {
		{
			name = "optional",
			check = function()
				return { state = "unavailable", detail = "not installed" }
			end,
		},
	},
})
assert(degraded.state == "degraded", "optional unavailable diagnostics must degrade rather than hide runtime health")

local failed = diagnostics.capture({
	runtime = owner,
	diagnostics = {
		{
			name = "failing",
			check = function()
				error("token: private-value")
			end,
		},
	},
})
assert(
	failed.state == "failed" and not failed.diagnostics[1].detail:find("private%-value"),
	"diagnostic failures must be explicit and redacted"
)

local cancelled = diagnostics.capture({
	runtime = owner,
	cancel = function()
		return true
	end,
})
assert(cancelled.state == "cancelled", "runtime snapshots must support cancellation")

local empty = diagnostics.capture({ runtime = runtime.new() })
assert(empty.state == "unavailable", "runtimes without services must remain explicitly unavailable")
