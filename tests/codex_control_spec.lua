local control = require("gator").module("adapters").codex_control
local calls = {}
local native = control.new({
	id = "codex-control",
	session = { provider = "codex", id = "thread-fixture", owner = "provider" },
	turn_id = "turn-fixture",
	request = function(request)
		table.insert(calls, request)
		if request.method == "turn/interrupt" then
			return { id = request.id, result = {} }
		end
		return { id = request.id, result = { thread = { id = "thread-fixture" } } }
	end,
})
assert(native:cancel("token=fixture-secret").state == "cancelled", "Codex cancellation must use native turn/interrupt")
assert(
	calls[1].method == "turn/interrupt"
		and calls[1].params.threadId == "thread-fixture"
		and calls[1].params.turnId == "turn-fixture"
		and native:recover().session.owner == "provider"
		and calls[2].method == "thread/resume",
	"Codex recovery must preserve the provider-owned native thread"
)

local failed = control.new({
	session = { provider = "codex", id = "thread-fixture", owner = "provider" },
	turn_id = "turn-fixture",
	request = function(request)
		return { id = request.id, error = { code = -32000, message = "token=fixture-secret" } }
	end,
})
assert(
	failed:cancel().state == "failed" and failed:status().reason == "token=[REDACTED]",
	"Codex cancellation failures must remain explicit and redacted"
)

local unavailable = control.unavailable("token=fixture-secret")
assert(
	unavailable:recover().state == "unavailable" and unavailable:status().reason == "token=[REDACTED]",
	"Codex control unavailability must remain explicit and redacted"
)

local mismatch = control.new({
	session = { provider = "codex", id = "thread-fixture", owner = "provider" },
	request = function(request)
		return { id = request.id, result = { thread = { id = "other-thread" } } }
	end,
})
assert(
	mismatch:recover().state == "failed",
	"Codex recovery must reject a native response that changes provider-owned session identity"
)
