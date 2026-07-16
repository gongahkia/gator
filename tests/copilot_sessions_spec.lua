local sessions = require("gator").module("adapters").copilot_sessions
local calls = {}
local function request(method, params)
	table.insert(calls, { method = method, params = params })
	return { sessionId = "copilot-new" }
end
assert(
	sessions.create({ request = request, cwd = "/tmp" }).id == "copilot-new",
	"Copilot session creation must preserve ACP session ids"
)
assert(calls[1].method == "session/new" and calls[1].params.mcpServers[1] == nil, "Copilot creation must use ACP")
assert(
	not sessions.list().available and not sessions.resume().available and not sessions.close().available,
	"unsupported Copilot history operations must remain explicit"
)
assert(#calls == 1, "Copilot must not fabricate session-load requests")
