local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local sessions = require("gator").module("adapters").amp_sessions
local stream = require("gator").module("adapters").amp_stream
local fixture = helpers.read(vim.g.gator_test.root .. "/tests/fixtures/adapters/amp_stream.jsonl")
local value = stream.parse(fixture)
assert(
	value.id == "T-amp-fixture" and value.result == "gator-fixture" and #value.events == 3,
	"Amp stream parser must preserve documented thread and completion records"
)

local calls = {}
local function run(argv)
	table.insert(calls, argv)
	if argv[2] == "threads" and argv[3] == "delete" then
		return { code = 0, stdout = "" }
	end
	return { code = 0, stdout = fixture }
end
assert(
	sessions.create({ run = run, prompt = "summarize" }).id == "T-amp-fixture",
	"Amp create must preserve native thread ids"
)
assert(
	sessions.resume({ run = run, id = "T-amp-fixture", prompt = "continue" }).id == "T-amp-fixture",
	"Amp resume must preserve native thread ids"
)
assert(
	calls[1][2] == "--execute"
		and calls[1][4] == "--stream-json"
		and calls[2][2] == "threads"
		and calls[2][3] == "continue"
		and calls[2][4] == "T-amp-fixture",
	"Amp sessions must use documented execute and thread continuation commands"
)
assert(sessions.close({ run = run, id = "T-amp-fixture" }), "Amp close must delete the provider-owned thread")
assert(
	not sessions.list().available,
	"Amp session listing must remain unavailable until the CLI documents a machine-readable schema"
)
assert(not pcall(sessions.create, { run = run, prompt = "" }), "Amp create must reject empty prompts")
