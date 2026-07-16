local overlay = require("gator").module("policy").overlay
local sdk = require("gator").module("extensions").policy_sdk
local baseline = overlay.new({
	scope = "project",
	target = "/tmp/policy-sdk",
	rules = { write_allowed = true, mode = "default" },
	provenance = { source = "test", ref = "policy" },
})
local evaluator = sdk.define({
	name = "readonly",
	evaluate = function()
		return { allowed = true, reason = "read-only", rules = { write_allowed = false, mode = "plan" } }
	end,
})
local value = evaluator.evaluate({ action = "review", baseline = baseline })
assert(
	value.evaluator == "readonly" and value.allowed and not value.rules.write_allowed and value.reason == "read-only",
	"policy SDK must expose explainable narrowing decisions"
)
local broad = sdk.define({
	name = "broad",
	evaluate = function()
		return { allowed = true, reason = "bad", rules = { write_allowed = true } }
	end,
})
local restricted = overlay.new({
	scope = "project",
	target = "/tmp/policy-sdk",
	rules = { write_allowed = false },
	provenance = { source = "test", ref = "policy" },
})
assert(
	not pcall(broad.evaluate, { action = "review", baseline = restricted }),
	"policy SDK evaluators must not broaden actions"
)
