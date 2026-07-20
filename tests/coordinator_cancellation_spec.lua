local gator = require("gator").setup()

local observed = {}
local operation = gator._coordinator:start_operation({
	id = "provider-stream",
	cancel = function(status)
		observed = status
	end,
})

assert(operation:status().active and not operation:status().cancelled, "coordinator operations must start active")
assert(
	gator.dispatch("cancel_operation", { id = "provider-stream", reason = "token: private-value" }),
	"dispatcher must route cancellation"
)
assert(
	operation:status().cancelled and observed.cancelled and observed.reason:find("private%-value") == nil,
	"coordinator cancellation must propagate redacted status to operation callbacks"
)
assert(not gator.dispatch("cancel_operation", { id = "provider-stream" }), "operation cancellation must be idempotent")
assert(operation:complete() and not operation:complete(), "completed operations must unregister exactly once")
assert(
	not pcall(gator.dispatch, "cancel_operation", { id = "provider-stream" }),
	"unavailable operations must fail explicitly"
)

local reconfigured = gator._coordinator:start_operation({ id = "configuration-change" })
gator.setup()
assert(reconfigured:status().cancelled, "setup must propagate cancellation to active coordinator operations")
