local faults = require("gator").module("core").transport_faults

local executed = 0
local value = faults.new({
	failures = {
		{
			operation = "connect",
			occurrence = 1,
			state = "unavailable",
			kind = "offline",
			reason = "transport is unavailable",
		},
		{
			operation = "read",
			occurrence = 2,
			state = "failed",
			kind = "timeout",
			reason = "token: private-value",
		},
	},
})
local unavailable = value:call("connect", function()
	executed = executed + 1
end)
assert(
	unavailable.state == "unavailable" and unavailable.kind == "offline" and executed == 0,
	"planned transport unavailability must bypass the runtime callback deterministically"
)
assert(value:call("read", function()
	return "first"
end).state == "ready", "unplanned transport calls must execute")
local failed = value:call("read", function()
	executed = executed + 1
end)
assert(
	failed.state == "failed" and failed.kind == "timeout" and not failed.reason:find("private%-value"),
	"planned transport failures must be indexed, explicit, and redacted"
)
assert(
	value:status().calls.read == 2 and value:status().triggered == 2,
	"fault controllers must retain deterministic call counts"
)

assert(value:cancel("test cancellation"), "transport fault controllers must support cancellation")
assert(
	value:call("read", function() end).state == "cancelled",
	"cancelled controllers must block later transport calls"
)

local callback_failed = faults.new({}):call("read", function()
	error("token: private-value")
end)
assert(
	callback_failed.state == "failed" and not callback_failed.reason:find("private%-value"),
	"runtime callback failures must remain deterministic and redacted"
)
