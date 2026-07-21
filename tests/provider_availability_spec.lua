local adapters = require("gator").module("adapters")
local availability = require("gator").module("core").provider_availability

local function contract(session, tool)
	local supported = { available = true, modes = { "native" } }
	return adapters.capabilities.new({
		provider = "codex",
		transport = supported,
		auth = supported,
		session = session,
		permission = supported,
		model = supported,
		command = supported,
		tool = tool,
		context = supported,
		usage = supported,
	})
end

local ready = availability.assess({
	capabilities = contract({ available = true, modes = { "resume" } }, { available = true, modes = { "native" } }),
	required = { { domain = "session", mode = "resume" } },
	optional = { { domain = "tool", mode = "native" } },
	probe = function(provider)
		return { available = provider == "codex" }
	end,
})
assert(
	ready.available and ready.state == "ready",
	"available required capabilities must produce a ready provider state"
)

local degraded = availability.assess({
	capabilities = contract(
		{ available = true, modes = { "resume" } },
		{ available = false, reason = "native tools unavailable" }
	),
	required = { { domain = "session", mode = "resume" } },
	optional = { { domain = "tool", mode = "native" } },
})
assert(
	degraded.available and degraded.state == "degraded" and degraded.degraded[1].domain == "tool",
	"optional provider capability loss must remain usable but degraded"
)

local unavailable = availability.assess({
	capabilities = contract(
		{ available = false, reason = "native sessions unavailable" },
		{ available = true, modes = { "native" } }
	),
	required = { { domain = "session", mode = "resume" } },
})
assert(
	not unavailable.available and unavailable.state == "unavailable",
	"required provider capability loss must be explicit"
)

local cancelled = availability.assess({
	capabilities = contract({ available = true, modes = { "resume" } }, { available = true, modes = { "native" } }),
	cancel = function()
		return true
	end,
})
assert(cancelled.state == "cancelled", "provider availability checks must support cancellation")

local failed = availability.assess({
	capabilities = contract({ available = true, modes = { "resume" } }, { available = true, modes = { "native" } }),
	probe = function()
		error("token: private-value")
	end,
})
assert(
	failed.state == "failed" and not failed.reason:find("private%-value"),
	"provider availability probe failures must be explicit and redacted"
)
