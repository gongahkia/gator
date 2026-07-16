local sessions = require("gator").module("adapters").codex_sessions
local calls = {}
local function request(method, params)
	table.insert(calls, { method = method, params = params })
	if method == "thread/start" then
		return { thread = { id = "thr-new" } }
	elseif method == "thread/list" then
		return { data = { { id = "thr-one" }, { id = "thr-two" } } }
	elseif method == "thread/resume" then
		return { thread = { id = params.threadId } }
	end
	return {}
end
assert(
	sessions.create({ request = request, cwd = "/tmp" }).id == "thr-new",
	"Codex session creation must use thread/start"
)
assert(#sessions.list({ request = request }) == 2, "Codex session listing must return provider-native thread ids")
assert(
	sessions.resume({ request = request, id = "thr-one" }).id == "thr-one",
	"Codex session resume must use thread/resume"
)
assert(sessions.close({ request = request, id = "thr-one" }), "Codex session close must unsubscribe natively")
assert(calls[4].method == "thread/unsubscribe", "Codex sessions must not fabricate unsupported close history")
local ok = pcall(sessions.list, {
	request = function()
		return { data = { { title = "no id" } } }
	end,
})
assert(not ok, "Codex session listing must reject missing native ids")
