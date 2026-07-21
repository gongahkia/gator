local graph = require("gator.health").graph

local checks = 0
local report = graph.evaluate({
	components = {
		{
			name = "workspace",
			depends_on = { "configuration" },
			check = function()
				checks = checks + 1
				return { status = "ready" }
			end,
		},
		{
			name = "configuration",
			depends_on = {},
			check = function()
				return { status = "ready" }
			end,
		},
		{
			name = "provider.codex",
			depends_on = { "workspace" },
			check = function()
				checks = checks + 1
				return { status = "unavailable", detail = "fixture provider is absent" }
			end,
		},
	},
})
assert(
	checks == 2
		and report.components[1].name == "configuration"
		and report.by_name.workspace.status == "ready"
		and report.by_name["provider.codex"].status == "unavailable",
	"health graphs must evaluate dependencies in deterministic order"
)

local blocked_calls = 0
local blocked = graph.evaluate({
	components = {
		{
			name = "handoff",
			depends_on = { "missing-provider" },
			check = function()
				blocked_calls = blocked_calls + 1
				return { status = "ready" }
			end,
		},
	},
})
assert(
	blocked.by_name.handoff.status == "unavailable" and blocked_calls == 0,
	"missing dependencies must make components explicitly unavailable without running their checks"
)

local failure = graph.evaluate({
	components = {
		{
			name = "provider.failure",
			depends_on = {},
			check = function()
				error("token: private-value")
			end,
		},
		{
			name = "dependent",
			depends_on = { "provider.failure" },
			check = function()
				return { status = "ready" }
			end,
		},
	},
	cancel = function(name)
		return name == "cancelled"
	end,
})
assert(
	failure.by_name["provider.failure"].status == "failed"
		and not failure.by_name["provider.failure"].detail:find("private%-value")
		and failure.by_name.dependent.status == "unavailable",
	"health graph failures must redact details and block dependents"
)

local cancelled = graph.evaluate({
	components = {
		{
			name = "cancelled",
			depends_on = {},
			check = function()
				return { status = "ready" }
			end,
		},
	},
	cancel = function()
		return true
	end,
})
assert(cancelled.by_name.cancelled.status == "cancelled", "health graph cancellation must be explicit")
assert(not pcall(graph.evaluate, {
	components = {
		{
			name = "first",
			depends_on = { "second" },
			check = function()
				return { status = "ready" }
			end,
		},
		{
			name = "second",
			depends_on = { "first" },
			check = function()
				return { status = "ready" }
			end,
		},
	},
}), "health graphs must reject cycles")
