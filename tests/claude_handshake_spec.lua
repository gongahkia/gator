local handshake = require("gator").module("adapters").claude_handshake
local command
local native = handshake.new({
	open = function(value)
		command = value
		return { type = "system", subtype = "init", session_id = "claude-fixture", apiKey = "not-retained" }
	end,
})
local connected = native:connect()
assert(
	connected.state == "ready" and connected.session.provider == "claude" and connected.session.id == "claude-fixture",
	"Claude handshake must retain only the provider-owned native session"
)
assert(
	vim.deep_equal(
		command,
		{ "claude", "-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose" }
	),
	"Claude handshake must use documented print-mode JSONL transport"
)

local failed = handshake.new({
	open = function()
		error("token=fixture-secret", 0)
	end,
})
assert(
	failed:connect().state == "failed" and failed:status().reason == "token=[REDACTED]",
	"Claude handshake failures must remain explicit and redacted"
)

local invalid = handshake.new({
	open = function()
		return { type = "assistant", session_id = "claude-fixture" }
	end,
})
assert(invalid:connect().state == "failed", "Claude handshake must reject non-init native events")

local unavailable = handshake.unavailable("token=fixture-secret")
assert(
	unavailable:connect().state == "unavailable" and unavailable:status().reason == "token=[REDACTED]",
	"Claude handshake unavailability must remain explicit and redacted"
)

local calls = 0
local cancelled = handshake.new({
	open = function()
		calls = calls + 1
	end,
})
assert(cancelled:cancel("token=fixture-secret"), "ready Claude handshakes must support cancellation")
assert(
	cancelled:connect().state == "cancelled" and cancelled:status().reason == "token=[REDACTED]" and calls == 0,
	"cancelled Claude handshakes must not open native transport"
)
