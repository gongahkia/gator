local process = require("gator").module("adapters").process
local callbacks = {}
local manager = process.new({
	spawn = function(_, _, callback)
		table.insert(callbacks, callback)
		return {
			pid = #callbacks,
			kill = function()
				return true
			end,
		}
	end,
})
local launched = manager:launch({ id = "agent-one", command = { "agent", "run" } })
assert(launched.state == "running" and launched.executable == "agent", "launch must expose only safe process metadata")
callbacks[1]({ code = 1, signal = 0 })
assert(manager:status("agent-one").state == "failed", "monitoring must record non-zero exits")
assert(manager:restart("agent-one").state == "running", "terminal processes must restart")
assert(manager:cancel("agent-one").state == "cancelling", "running processes must support cancellation")
callbacks[2]({ code = 0, signal = 15 })
assert(manager:status("agent-one").state == "cancelled", "cancelled processes must not report successful completion")
assert(manager:cleanup("agent-one") and not manager:status("agent-one"), "terminal processes must clean up")
local ok = pcall(manager.restart, manager, "missing")
assert(not ok, "unknown processes must fail explicitly")
