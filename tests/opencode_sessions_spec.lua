local sessions = require("gator").module("adapters").opencode_sessions
local calls = {}
local function request(method, params)
	table.insert(calls, { method = method, params = params })
	if method == "session/new" then
		return { sessionId = "opencode-new" }
	end
	if method == "session/list" then
		return { sessions = { { sessionId = "opencode-new" }, { sessionId = "opencode-old" } } }
	end
	return {}
end
assert(
	sessions.create({ request = request, cwd = "/tmp" }).id == "opencode-new",
	"OpenCode session creation must preserve ACP session ids"
)
local listed = sessions.list({ request = request, cwd = "/tmp" })
assert(
	#listed == 2 and listed[2].id == "opencode-old" and listed[1].owner == "provider",
	"OpenCode session listing must preserve provider-owned history"
)
assert(
	sessions.resume({ request = request, id = "opencode-new", cwd = "/tmp" }).id == "opencode-new",
	"OpenCode session resume must preserve the provider session id"
)
assert(sessions.close({ request = request, id = "opencode-new" }), "OpenCode session close must confirm ACP success")
assert(
	calls[1].method == "session/new"
		and calls[1].params.mcpServers[1] == nil
		and calls[2].method == "session/list"
		and calls[2].params.cwd == "/tmp"
		and calls[3].method == "session/resume"
		and calls[3].params.mcpServers[1] == nil
		and calls[4].method == "session/close",
	"OpenCode sessions must use only advertised ACP lifecycle methods"
)
assert(not pcall(sessions.resume, { request = request, id = "opencode-new" }), "OpenCode resume must require cwd")
assert(not pcall(sessions.close, { request = request, id = "" }), "OpenCode close must require an opaque id")
