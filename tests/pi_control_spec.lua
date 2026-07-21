local control = require("gator").module("adapters").pi_control
local calls = {}
local native = control.new({
	id = "pi-control",
	request = function(request)
		table.insert(calls, request)
		if request.type == "abort" then
			return { id = request.id, type = "response", command = "abort", success = true }
		end
		return {
			id = request.id,
			type = "response",
			command = "get_state",
			success = true,
			data = { isStreaming = false, sessionFile = "/tmp/pi-fixture.jsonl" },
		}
	end,
})
assert(native:cancel("token=fixture-secret").state == "cancelled", "Pi cancellation must use the native abort command")
assert(
	calls[1].type == "abort" and native:recover().session.owner == "provider" and calls[2].type == "get_state",
	"Pi recovery must preserve provider-owned session paths"
)

local failed = control.new({
	request = function(request)
		return {
			id = request.id,
			type = "response",
			command = request.type,
			success = false,
			error = "token=fixture-secret",
		}
	end,
})
assert(
	failed:cancel().state == "failed" and failed:status().reason == "token=[REDACTED]",
	"Pi cancellation failures must remain explicit and redacted"
)

local unavailable = control.unavailable("token=fixture-secret")
assert(
	unavailable:recover().state == "unavailable" and unavailable:status().reason == "token=[REDACTED]",
	"Pi control unavailability must remain explicit and redacted"
)
