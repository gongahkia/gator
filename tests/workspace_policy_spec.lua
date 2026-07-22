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
local write = overlay.new({
	scope = "run",
	target = "run-write",
	rules = { write_allowed = true },
	provenance = { source = "run-override", ref = "run-write" },
})
assert(
	policy.preflight({ policy = write, write = true, approval = { state = "approved" } }).state == "completed",
	"write preflight must require and accept explicit native approval"
)
assert(
	policy.preflight({ policy = write, write = true, approval = { state = "pending" } }).state == "unavailable",
	"pending native approval must suppress provider launch"
)
assert(
	policy.preflight({ policy = write, write = true, approval = { state = "denied", reason = "user declined" } }).state
		== "failed",
	"denied native approval must fail explicitly"
)
local read_only = overlay.new({
	scope = "run",
	target = "run-read-only",
	rules = { write_allowed = false },
	provenance = { source = "run-override", ref = "run-read-only" },
})
assert(
	policy.preflight({ policy = read_only, write = true, approval = { state = "approved" } }).state == "unavailable",
	"write policy must reject writable launch before provider invocation"
)
assert(policy.preflight({
	policy = write,
	write = true,
	approval = { state = "approved" },
	cancelled = function()
		return true
	end,
}).state == "cancelled", "cancelled preflight must not launch a provider")
local calls = 0
local blocked = policy.launch({
	policy = write,
	write = true,
	approval = { state = "pending" },
	launch = function()
		calls = calls + 1
	end,
})
assert(
	blocked.state == "unavailable" and calls == 0,
	"unapproved writes must never invoke the provider-native launcher"
)
local session = { id = "provider-native-session" }
local launched = policy.launch({
	policy = write,
	write = true,
	approval = { state = "granted" },
	launch = function()
		calls = calls + 1
		return session
	end,
})
assert(
	launched.state == "completed" and launched.launch == session and calls == 1,
	"approved launches must preserve provider-native session ownership"
)
