local coordinator = require("gator.coordinator")
local consent = require("gator.telemetry.consent")
local motion = require("gator.ui.motion")
local redact = require("gator.policy.redact")

local value = coordinator.new({
	ui = { motion = { enabled = false, interval_ms = 16, reduced = false } },
	telemetry = { enabled = true, redaction_patterns = { "private%-[%w]+" } },
})

assert(coordinator.is(value), "coordinator must expose a typed production composition root")
assert(
	value:state().config.telemetry.enabled and value:state().compatibility.supported,
	"coordinator must retain validated settings and compatibility"
)
assert(not motion.active(), "coordinator must configure UI motion from validated settings")
assert(consent.status().enabled, "coordinator must configure telemetry consent from validated settings")
assert(redact.text("private-value") == "[REDACTED]", "coordinator must configure configured redaction")
assert(not pcall(coordinator.new, { unsupported = true }), "coordinator must reject unsupported settings")
assert(not pcall(value.state, {}), "coordinator methods must reject invalid receivers")
assert(value:module("core").name == "core", "coordinator must resolve public modules through its container")

local operation = value:start_operation({ id = "inspection-run", key = "inspection-run-key", kind = "launch" })
value:cancel_operation("inspection-run", "token: private-value")
local inspection = value:inspect()
assert(
	inspection.schema_version == 1
		and inspection.state_version == 0
		and inspection.state.config.telemetry.enabled
		and inspection.operations[1].state == "cancelled"
		and inspection.operations[1].reason:find("private%-value") == nil,
	"coordinator inspection must expose versioned, redacted state and operation snapshots"
)
inspection.state.config.telemetry.enabled = false
assert(value:inspect().state.config.telemetry.enabled, "coordinator inspection snapshots must not mutate live state")
assert(not pcall(value.inspect, {}), "coordinator inspection must reject invalid receivers")

coordinator.new()
