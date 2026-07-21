local handshake = require("gator").module("adapters").gemini_handshake
local request
local native = handshake.new({
	id = "gemini-handshake",
	client_info = { name = "gator", title = "Gator", version = "1" },
	request = function(value)
		request = value
		return {
			id = "gemini-handshake",
			result = {
				protocolVersion = 1,
				authMethods = { { id = "oauth-personal" } },
				agentInfo = { name = "gemini-cli", title = "Gemini CLI", version = "0.46.0" },
				agentCapabilities = {
					loadSession = true,
					mcpCapabilities = { http = true, sse = true },
					promptCapabilities = { image = true, audio = true, embeddedContext = true },
				},
			},
		}
	end,
})
local connected = native:connect()
assert(
	connected.state == "ready"
		and connected.agent.name == "gemini-cli"
		and connected.capabilities.load_session
		and connected.capabilities.embedded_context
		and connected.authMethods == nil,
	"Gemini handshakes must retain only safe native agent metadata and capabilities"
)
assert(
	request.method == "initialize"
		and request.params.protocolVersion == 1
		and request.params.clientInfo.name == "gator"
		and vim.deep_equal(native.command, { "gemini", "--acp" }),
	"Gemini handshakes must use the native ACP initialize request"
)

local failed = handshake.new({
	request = function()
		return { id = "gator-handshake", error = { code = -32000, message = "token=fixture-secret" } }
	end,
})
assert(
	failed:connect().state == "failed" and failed:status().reason == "token=[REDACTED]",
	"Gemini handshake failures must remain explicit and redacted"
)

local invalid = handshake.new({
	request = function()
		return { id = "gator-handshake", result = { protocolVersion = 2 } }
	end,
})
assert(invalid:connect().state == "failed", "Gemini handshakes must reject unsupported ACP profiles")

local unavailable = handshake.unavailable("token=fixture-secret")
assert(
	unavailable:connect().state == "unavailable" and unavailable:status().reason == "token=[REDACTED]",
	"Gemini handshake unavailability must remain explicit and redacted"
)

local calls = 0
local cancelled = handshake.new({
	request = function()
		calls = calls + 1
	end,
})
assert(cancelled:cancel("token=fixture-secret"), "ready Gemini handshakes must support cancellation")
assert(
	cancelled:connect().state == "cancelled" and cancelled:status().reason == "token=[REDACTED]" and calls == 0,
	"cancelled Gemini handshakes must not send a native initialize request"
)
