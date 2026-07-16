local sessions = require("gator").module("adapters").copilot_sessions
local calls = {}
local function request(method, params)
	table.insert(calls, { method = method, params = params })
	if method == "session/new" then
		return { sessionId = "copilot-new" }
	end
	return { sessionId = params.sessionId }
end
assert(
	sessions.create({ request = request, cwd = "/tmp" }).id == "copilot-new",
	"Copilot session creation must preserve ACP session ids"
)
assert(
	sessions.resume({ request = request, id = "copilot-new", cwd = "/tmp" }).id == "copilot-new",
	"Copilot session resume must preserve ACP session ids"
)
assert(calls[1].method == "session/new" and calls[1].params.mcpServers[1] == nil, "Copilot creation must use ACP")
assert(
	calls[2].method == "session/load" and calls[2].params.cwd == "/tmp",
	"Copilot resume must load with explicit cwd"
)
assert(
	not sessions.list().available and not sessions.close().available,
	"unsupported Copilot history operations must remain explicit"
)
assert(not pcall(sessions.resume, { request = request, id = "copilot-new" }), "Copilot resume must require cwd")
