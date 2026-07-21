local control = require("gator").module("adapters").claude_control
local calls = {}
local native = control.new({
	session = { provider = "claude", id = "claude-fixture", owner = "provider" },
	interrupt = function()
		table.insert(calls, "interrupt")
	end,
	resume = function(id)
		table.insert(calls, id)
		return { type = "system", subtype = "init", session_id = "claude-fixture" }
	end,
})
assert(
	native:cancel("token=fixture-secret").state == "cancelled",
	"Claude cancellation must use native query interruption"
)
assert(
	calls[1] == "interrupt" and native:recover().session.owner == "provider" and calls[2] == "claude-fixture",
	"Claude recovery must resume the provider-owned native session"
)

local failed = control.new({
	session = { provider = "claude", id = "claude-fixture", owner = "provider" },
	interrupt = function()
		error("token=fixture-secret", 0)
	end,
	resume = function()
		return { type = "system", subtype = "init", session_id = "claude-fixture" }
	end,
})
assert(
	failed:cancel().state == "failed" and failed:status().reason == "token=[REDACTED]",
	"Claude cancellation failures must remain explicit and redacted"
)

local unavailable = control.unavailable("token=fixture-secret")
assert(
	unavailable:recover().state == "unavailable" and unavailable:status().reason == "token=[REDACTED]",
	"Claude control unavailability must remain explicit and redacted"
)

local mismatch = control.new({
	session = { provider = "claude", id = "claude-fixture", owner = "provider" },
	interrupt = function() end,
	resume = function()
		return { type = "system", subtype = "init", session_id = "other-session" }
	end,
})
assert(mismatch:recover().state == "failed", "Claude recovery must reject changed provider session identity")
