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
	consent = consent,
})
local seen = {}
for _, record in ipairs(records) do
	seen[record.component] = record
end
assert(
	seen["adapter.codex"].level == "ok" and seen["adapter.claude"].level == "warn",
	"health readiness must report every adapter without launching agents"
)
assert(
	seen.git.level == "ok" and seen.indexer.level == "ok" and seen.workspace.level == "ok",
	"health readiness must report Git, indexer, and workspace state"
)
assert(
	seen.policy.level == "ok" and seen.telemetry.level == "ok" and seen.telemetry.message:find("disabled", 1, true),
	"health readiness must report valid policy and default-denied telemetry consent"
)
assert(not pcall(health.readiness, { executable = true }), "invalid readiness probes must fail explicitly")
