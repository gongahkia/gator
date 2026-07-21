local handshake = require("gator").module("adapters").opencode_handshake
local request
local native = handshake.new({
	id = "opencode-handshake",
	cwd = vim.g.gator_test.root,
	client_info = { name = "gator", title = "Gator", version = "1" },
	request = function(value)
		request = value
		return {
			id = "opencode-handshake",
			result = {
				protocolVersion = 1,
				authMethods = { { id = "opencode-login", description = "native only" } },
				agentInfo = { name = "OpenCode", version = "1.17.15" },
				agentCapabilities = {
					loadSession = true,
					mcpCapabilities = { http = true, sse = true },
					promptCapabilities = { embeddedContext = true, image = true },
					sessionCapabilities = { close = {}, fork = {}, list = {}, resume = {} },
				},
			},
		}
	end,
})
local connected = native:connect()
assert(
	connected.state == "ready"
		and connected.agent.name == "OpenCode"
		and connected.capabilities.load_session
		and connected.capabilities.session_resume
		and connected.capabilities.embedded_context
		and connected.authMethods == nil,
	"OpenCode handshakes must retain only safe native metadata and capabilities"
)
assert(
	request.method == "initialize"
		and request.params.protocolVersion == 1
		and request.params.clientInfo.name == "gator"
		and vim.deep_equal(native.command, { "opencode", "acp", "--cwd", vim.g.gator_test.root }),
	"OpenCode handshakes must use native ACP initialization in the requested directory"
)

local failed = handshake.new({
	request = function()
		return { id = "gator-handshake", error = { code = -32000, message = "token=fixture-secret" } }
	end,
})
assert(
	failed:connect().state == "failed" and failed:status().reason == "token=[REDACTED]",
	"OpenCode handshake failures must remain explicit and redacted"
)

local invalid = handshake.new({
	request = function()
		return { id = "gator-handshake", result = { protocolVersion = 2 } }
	end,
})
assert(invalid:connect().state == "failed", "OpenCode handshakes must reject unsupported ACP profiles")

local unavailable = handshake.unavailable("token=fixture-secret")
assert(
	unavailable:connect().state == "unavailable" and unavailable:status().reason == "token=[REDACTED]",
	"OpenCode handshake unavailability must remain explicit and redacted"
)

local calls = 0
local cancelled = handshake.new({
	request = function()
		calls = calls + 1
	end,
})
assert(cancelled:cancel("token=fixture-secret"), "ready OpenCode handshakes must support cancellation")
assert(
	cancelled:connect().state == "cancelled" and cancelled:status().reason == "token=[REDACTED]" and calls == 0,
	"cancelled OpenCode handshakes must not send a native initialize request"
)
