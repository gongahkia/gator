local handshake = require("gator").module("adapters").pi_handshake
local request
local native = handshake.new({
	id = "pi-handshake",
	request = function(value)
		request = value
		return {
			id = "pi-handshake",
			type = "response",
			command = "get_state",
			success = true,
			data = { sessionId = "pi-fixture", sessionFile = "/tmp/pi-fixture.jsonl", isStreaming = false },
		}
	end,
})
local connected = native:connect()
assert(
	connected.state == "ready"
		and not connected.streaming
		and connected.session.id == "/tmp/pi-fixture.jsonl"
		and connected.session.owner == "provider",
	"Pi handshake must preserve provider-owned session references"
)
assert(
	request.type == "get_state" and request.id == "pi-handshake",
	"Pi handshake must use the native get_state request"
)

local failed = handshake.new({
	request = function()
		return {
			id = "gator-handshake",
			type = "response",
			command = "get_state",
			success = false,
			error = "token=fixture-secret",
		}
	end,
})
assert(
	failed:connect().state == "failed" and failed:status().reason == "token=[REDACTED]",
	"Pi handshake failures must remain explicit and redacted"
)

local unavailable = handshake.unavailable("token=fixture-secret")
assert(
	unavailable:connect().state == "unavailable" and unavailable:status().reason == "token=[REDACTED]",
	"Pi handshake unavailability must remain explicit and redacted"
)

local calls = 0
local cancelled = handshake.new({
	request = function()
		calls = calls + 1
	end,
})
assert(cancelled:cancel("token=fixture-secret"), "ready Pi handshakes must support cancellation")
assert(
	cancelled:connect().state == "cancelled" and cancelled:status().reason == "token=[REDACTED]" and calls == 0,
	"cancelled Pi handshakes must not send a native request"
)
