local sessions = require("gator").module("adapters").claude_sessions
local calls = {}
local function run(argv)
	table.insert(calls, argv)
	return { code = 0, stdout = '{"type":"result","is_error":false,"session_id":"claude-one"}' }
end
assert(
	sessions.create({ run = run, prompt = "summarize" }).id == "claude-one",
	"Claude create must preserve native JSON session ids"
)
assert(
	sessions.resume({ run = run, id = "claude-one", prompt = "continue" }).id == "claude-one",
	"Claude resume must use the native session id"
)
assert(
	calls[2][3] == "--resume" and calls[2][4] == "claude-one",
	"Claude resume must invoke the documented resume flag"
)
assert(
	not sessions.list().available and not sessions.close().available,
	"unsupported Claude history operations must remain explicit"
)
local ok = pcall(sessions.create, { run = run, prompt = "" })
assert(not ok, "empty Claude prompts must fail explicitly")
