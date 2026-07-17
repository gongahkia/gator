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
local launched = manager:launch({ id = "agent-one", command = { "agent", "run" }, timeout_ms = 5000 })
assert(launched.state == "running" and launched.executable == "agent", "launch must expose only safe process metadata")
assert(launched.timeout_ms == 5000, "managed processes must retain bounded runtime timeouts")
callbacks[1]({ code = 1, signal = 0 })
assert(manager:status("agent-one").state == "failed", "monitoring must record non-zero exits")
assert(manager:restart("agent-one").state == "running", "terminal processes must restart")
assert(manager:cancel("agent-one").state == "cancelling", "running processes must support cancellation")
callbacks[2]({ code = 0, signal = 15 })
assert(manager:status("agent-one").state == "cancelled", "cancelled processes must not report successful completion")
assert(manager:cleanup("agent-one") and not manager:status("agent-one"), "terminal processes must clean up")
local ok = pcall(manager.restart, manager, "missing")
assert(not ok, "unknown processes must fail explicitly")
assert(
	not pcall(manager.launch, manager, { id = "bad-timeout", command = { "agent" }, timeout_ms = 0 }),
	"invalid timeouts must fail explicitly"
)
