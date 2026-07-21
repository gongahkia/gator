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

local idempotent = gator._coordinator:start_operation({
	id = "launch-one",
	key = "launch-retry-one",
	kind = "launch",
})
local replayed = gator._coordinator:start_operation({
	id = "launch-retry",
	key = "launch-retry-one",
	kind = "launch",
})
assert(
	replayed == idempotent and idempotent:status().key == "launch-retry-one" and idempotent:status().kind == "launch",
	"matching in-flight launch keys must return the original operation"
)
assert(not pcall(gator._coordinator.start_operation, gator._coordinator, {
	id = "handoff-one",
	key = "launch-retry-one",
	kind = "handoff",
}), "operation keys must reject cross-kind retries")
assert(idempotent:complete(), "idempotent operations must complete once")
assert(
	gator._coordinator:start_operation({ id = "launch-retry-after-complete", key = "launch-retry-one", kind = "launch" })
		== idempotent,
	"completed operation keys must retain their original outcome"
)
assert(idempotent:status().state == "completed", "completed operation keys must expose a terminal outcome")
assert(
	gator._coordinator:start_operation({ id = "launch-next", key = "launch-retry-two", kind = "launch" }):complete(),
	"new launches must use a distinct idempotency key"
)
local handoff_callbacks = 0
local handoff = gator._coordinator:start_operation({
	id = "handoff-one",
	key = "handoff-retry-one",
	kind = "handoff",
	cancel = function()
		handoff_callbacks = handoff_callbacks + 1
	end,
})
assert(
	gator._coordinator:start_operation({ id = "handoff-retry", key = "handoff-retry-one", kind = "handoff" }) == handoff
		and gator.dispatch("cancel_operation", { id = "handoff-one" })
		and handoff_callbacks == 1
		and handoff:status().state == "cancelled",
	"keyed handoff retries must preserve one cancellation outcome"
)
assert(handoff:complete(), "cancelled handoffs must complete cleanup once")
assert(
	not pcall(gator._coordinator.start_operation, gator._coordinator, { id = "launch-secret", key = "ghp_secret" }),
	"operation keys must reject sensitive values"
)
