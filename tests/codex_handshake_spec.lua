local handshake = require("gator").module("adapters").codex_handshake
local request
local native = handshake.new({
	id = "codex-handshake",
	client_info = { name = "gator", title = "Gator", version = "1" },
	request = function(value)
		request = value
		return {
			id = "codex-handshake",
			result = {
				userAgent = "gator-fixture/0.144",
				codexHome = "/tmp/gator-codex",
				platformFamily = "unix",
				platformOs = "macos",
			},
		}
	end,
})
local connected = native:connect()
assert(
	connected.state == "ready"
		and connected.platform_family == "unix"
		and connected.platform_os == "macos"
		and connected.userAgent == nil
		and connected.codexHome == nil,
	"Codex handshakes must retain only non-sensitive native platform metadata"
)
assert(
	request.id == "codex-handshake"
		and request.method == "initialize"
		and request.params.clientInfo.name == "gator"
		and request.params.clientInfo.title == "Gator",
	"Codex handshakes must use the native initialize request"
)

local current = handshake.new({
	request = function()
		return {
			id = "gator-handshake",
			result = { userAgent = "gator-fixture/0.145", platformFamily = "unix", platformOs = "macos" },
		}
	end,
})
assert(
	current:connect().state == "ready",
	"Codex handshakes must accept the current initialize response without codexHome"
)

local failed = handshake.new({
	request = function()
		return { id = "gator-handshake", error = { code = -32000, message = "token=fixture-secret" } }
	end,
})
assert(
	failed:connect().state == "failed" and failed:status().reason == "token=[REDACTED]",
	"Codex handshake failures must remain explicit and redacted"
)

local unavailable = handshake.unavailable("token=fixture-secret")
assert(
	unavailable:connect().state == "unavailable" and unavailable:status().reason == "token=[REDACTED]",
	"Codex handshake unavailability must remain explicit and redacted"
)

local calls = 0
local cancelled = handshake.new({
	request = function()
		calls = calls + 1
	end,
})
assert(cancelled:cancel("token=fixture-secret"), "ready Codex handshakes must support cancellation")
assert(
	cancelled:connect().state == "cancelled" and cancelled:status().reason == "token=[REDACTED]" and calls == 0,
	"cancelled Codex handshakes must not send a native initialize request"
)
