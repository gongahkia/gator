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
	sessions.resume({ request = request, cwd = "/tmp", id = "copilot-new" }).id == "copilot-new"
		and calls[2].method == "session/load"
		and calls[2].params.sessionId == "copilot-new",
	"Copilot session reattach must dynamically use ACP session loading"
)
assert(
	not sessions.list().available and not sessions.close().available and #calls == 2,
	"Copilot history listing and deletion must remain explicit"
)
