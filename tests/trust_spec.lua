local config = require("gator.config")
local trust = require("gator.trust")

local settings = config.resolve()
local codex = trust.resolve({ provider = "codex", transport = "chat", config = settings })
assert(
	codex.surface == "structured"
		and codex.write.state == "codex_enforced"
		and codex.write.mode == "workspace_write"
		and codex.approval.state == "on_request"
		and vim.deep_equal(trust.codex_policy(codex), { sandbox = "workspace-write", approval_policy = "on-request" }),
	"Codex chat trust must record and reconstruct the enforced App Server policy"
)

local read_only = trust.resolve({
	provider = "codex",
	transport = "chat",
	config = settings,
	project_policy = { available = true, policy = { rules = { write_allowed = false } } },
})
assert(
	read_only.write.mode == "read_only"
		and read_only.policy.mode == "project-policy"
		and trust.codex_policy(read_only).sandbox == "read-only",
	"a tracked read-only project policy must override the configured Codex default"
)

local terminal = trust.resolve({ provider = "codex", transport = "terminal", config = settings })
assert(
	terminal.write.state == "provider_owned"
		and terminal.network.state == "provider_owned"
		and terminal.approval.state == "provider_owned",
	"terminal runs must state that the provider owns permissions"
)

local pi = trust.resolve({ provider = "pi", transport = "chat", config = settings })
assert(
	pi.write.state == "unknown" and pi.approval.state == "unavailable",
	"Pi RPC must not be represented as a sandboxed or approval-mediated provider"
)

local managed = trust.resolve({ provider = "gemini", transport = "chat", config = settings })
assert(
	managed.write.state == "unknown" and managed.approval.state == "when_emitted",
	"managed agents must expose only approval requests they actually emit"
)

assert(not pcall(trust.resolve, {
	provider = "pi",
	transport = "chat",
	config = settings,
	project_policy = { available = true, policy = { rules = { write_allowed = false } } },
}), "a read-only project policy must reject structured providers Gator cannot constrain")
assert(not pcall(trust.resolve, {
	provider = "codex",
	transport = "terminal",
	config = settings,
	project_policy = { available = true, policy = { rules = { write_allowed = false } } },
}), "a read-only project policy must reject terminal providers Gator cannot constrain")

local legacy = trust.normalize(nil, { provider = "codex", transport = "chat" })
assert(
	legacy.write.state == "unknown" and trust.codex_policy(legacy) == nil,
	"legacy run records must remain readable without fabricated enforcement"
)
