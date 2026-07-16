local sessions = require("gator").module("adapters").gemini_sessions
local calls = {}
local function run(argv)
	table.insert(calls, argv)
	if argv[2] == "--list-sessions" then
		return { code = 0, stdout = "Available sessions for this project (1):\n  1. Fix adapter (now) [gemini-one]\n" }
	end
	if argv[2] == "--delete-session" then
		return { code = 0, stdout = "Deleted session 1: Fix adapter (now)" }
	end
	return { code = 0, stdout = '{"type":"init","session_id":"gemini-one"}\n{"type":"result","status":"success"}\n' }
end
assert(
	sessions.create({ run = run, prompt = "start" }).id == "gemini-one",
	"Gemini create must preserve native session ids"
)
assert(sessions.list({ run = run })[1].id == "gemini-one", "Gemini list must return native sessions")
assert(
	sessions.resume({ run = run, id = "gemini-one", prompt = "continue" }).id == "gemini-one",
	"Gemini resume must preserve native ids"
)
assert(sessions.close({ run = run, id = "gemini-one" }), "Gemini close must use native deletion")
assert(calls[4][2] == "--delete-session", "Gemini close must not fabricate unsupported history")
assert(calls[1][5] == "stream-json", "Gemini create must request native session init events")
assert(calls[3][7] == "stream-json", "Gemini resume must request native session init events")
local listed = sessions.list({
	run = function()
		return { code = 0, stdout = "No previous sessions found for this project." }
	end,
})
assert(#listed == 0, "Gemini list must preserve an explicitly empty native history")
assert(not pcall(sessions.list, {
	run = function()
		return { code = 0, stdout = "unknown output" }
	end,
}), "Gemini list must reject unsupported output")
assert(not pcall(sessions.close, {
	run = function()
		return { code = 0, stdout = "Cannot delete the current active session." }
	end,
	id = "gemini-one",
}), "Gemini close must require native deletion confirmation")
