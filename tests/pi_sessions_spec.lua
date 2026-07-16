local sessions = require("gator").module("adapters").pi_sessions
local calls = {}
local function request(command)
	table.insert(calls, command)
	if command.type == "new_session" then
		return { type = "response", command = "new_session", success = true, data = { cancelled = false } }
	end
	if command.type == "switch_session" then
		return { type = "response", command = "switch_session", success = true, data = { cancelled = false } }
	end
	return {
		type = "response",
		command = "get_state",
		success = true,
		data = { sessionId = "pi-fixture", sessionFile = "/tmp/pi-fixture.jsonl" },
	}
end
assert(
	sessions.create({ request = request }).id == "/tmp/pi-fixture.jsonl",
	"Pi session creation must preserve the provider resume path"
)
assert(
	sessions.resume({ request = request, id = "/tmp/pi-fixture.jsonl" }).id == "/tmp/pi-fixture.jsonl",
	"Pi session resume must preserve the provider resume path"
)
assert(
	calls[1].type == "new_session"
		and calls[2].type == "get_state"
		and calls[3].type == "switch_session"
		and calls[3].sessionPath == "/tmp/pi-fixture.jsonl"
		and calls[4].type == "get_state",
	"Pi sessions must use only advertised RPC lifecycle methods"
)
assert(
	not sessions.list().available and not sessions.close().available,
	"unsupported Pi session history operations must remain explicit"
)
assert(not pcall(sessions.resume, { request = request, id = "" }), "Pi resume must require a provider session path")
