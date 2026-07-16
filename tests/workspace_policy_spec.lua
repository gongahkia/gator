local overlay = require("gator").module("policy").overlay
local policy = require("gator").module("workspace").policy
local current = overlay.new({
	scope = "project",
	target = "project",
	rules = { workspace = "current" },
	provenance = { source = "project-policy", ref = ".gator/policy.json" },
})
assert(policy.resolve(current).kind == "project", "current workspace policy must select project checkout")
local worktree = overlay.new({
	scope = "project",
	target = "project",
	rules = { workspace = "worktree" },
	provenance = { source = "project-policy", ref = ".gator/policy.json" },
})
assert(policy.resolve(worktree).kind == "worktree", "worktree policy must select isolated checkout")
local invalid = overlay.new({
	scope = "project",
	target = "project",
	rules = { workspace = "automatic" },
	provenance = { source = "project-policy", ref = ".gator/policy.json" },
})
assert(not pcall(policy.resolve, invalid), "workspace policy must reject unavailable behavior")
