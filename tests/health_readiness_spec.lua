local health = require("gator.health")
local consent = {
	status = function()
		return { enabled = false }
	end,
}
local records = health.readiness({
	executable = function(name)
		return name == "git" or name == "codex" or name == "gator-index"
	end,
	cwd = "/fixture",
	run = function()
		return { code = 0, stdout = "true\n" }
	end,
	probe = function(provider)
		return { provider = provider, available = true, supported = true, version = "1.0.0" }
	end,
	auth = function(provider)
		return { provider = provider, authenticated = true }
	end,
	consent = consent,
})
local seen = {}
for _, record in ipairs(records) do
	seen[record.component] = record
end
assert(
	seen["adapter.codex"].level == "ok" and seen["adapter.claude"].level == "warn",
	"health readiness must report every adapter with bounded capability probes"
)
assert(
	seen.git.level == "ok" and seen.indexer.level == "ok" and seen.workspace.level == "ok",
	"health readiness must report Git, indexer, and workspace state"
)
assert(
	seen.policy.level == "ok" and seen.telemetry.level == "ok" and seen.telemetry.message:find("disabled", 1, true),
	"health readiness must report valid policy and default-denied telemetry consent"
)
local unverified = health.readiness({
	executable = function(name)
		return name == "codex"
	end,
	probe = function()
		return { available = true, supported = true, version = "1.0.0" }
	end,
	auth = function()
		return { authenticated = false, reason = "fixture login is absent" }
	end,
	consent = consent,
})
local codex
for _, record in ipairs(unverified) do
	if record.component == "adapter.codex" then
		codex = record
	end
end
assert(
	codex.level == "warn" and codex.message:find("authentication not verified", 1, true),
	"health must not report provider readiness from executable and capability presence alone"
)
local confirmed = health.readiness({
	executable = function(name)
		return name == "droid"
	end,
	probe = function()
		return { available = true, supported = true, version = "1.0.0" }
	end,
	auth = function()
		return { authenticated = false, reason = "Droid exposes no credential-status command" }
	end,
	settings = { providers = { droid = { user_confirmed = true } } },
	consent = consent,
})
local droid
for _, record in ipairs(confirmed) do
	if record.component == "adapter.droid" then
		droid = record
	end
end
assert(
	droid.readiness_state == "user_confirmed" and droid.message:find("credentials not verified", 1, true),
	"health must distinguish explicit readiness confirmation from credential verification"
)
assert(
	not pcall(health.readiness, { executable = true }) and not pcall(health.readiness, { auth = true }),
	"invalid readiness probes must fail explicitly"
)
