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

coordinator.new()
