local consent = require("gator").module("telemetry").consent
assert(not consent.status().enabled, "telemetry consent must default to disabled")
assert(not pcall(consent.require, "collection"), "telemetry collection must require explicit consent")
assert(not pcall(consent.require, "transmission"), "telemetry transmission must require explicit consent")
assert(consent.configure({ enabled = true }).enabled, "explicit telemetry opt-in must be retained")
assert(consent.require("collection") and consent.require("transmission"), "enabled consent must gate telemetry actions")
assert(not pcall(consent.configure, { enabled = "yes" }), "invalid telemetry consent must fail explicitly")
consent.configure({ enabled = false })
