local overlay = require("gator").module("policy").overlay
local global = overlay.new({
	scope = "global",
	rules = { workspace_mode = "project", write_runs = { maximum = 1 } },
	provenance = { source = "user-config", ref = "~/.config/nvim/gator.lua" },
})
local file = overlay.new({
	scope = "file",
	target = "lua/gator/**",
	rules = { write_allowed = false },
	provenance = { source = "project-policy", ref = ".gator/policy.json" },
})

assert(overlay.is(global) and overlay.is(file), "policy overlays must have a distinct entity type")
assert(global.rules.write_runs.maximum == 1, "policy overlays must preserve structured rules")
assert(file.target == "lua/gator/**", "scoped overlays must preserve their target")
assert(file.provenance.source == "project-policy", "policy overlays must preserve provenance")

local record = overlay.to_record(file)
record.rules.write_allowed = true
assert(not file.rules.write_allowed, "policy records must not mutate overlays")
assert(overlay.from_record(overlay.to_record(global)).scope == "global", "policy overlays must round-trip")

local ok = pcall(overlay.new, {
	scope = "global",
	target = "invalid",
	rules = {},
	provenance = { source = "user", ref = "config" },
})
assert(not ok, "global overlays must reject targets")
ok = pcall(overlay.new, {
	scope = "repository",
	rules = { token = "credential" },
	provenance = { source = "project", ref = "policy" },
})
assert(not ok, "policy overlays must reject credentials")
