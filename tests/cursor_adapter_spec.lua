local adapters = require("gator").module("adapters")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local policy = require("gator").module("policy").overlay
local fixture = helpers.read(vim.g.gator_test.root .. "/tests/fixtures/adapters/cursor_stream.jsonl")
local stream = adapters.cursor_stream.parse(fixture)
assert(
	stream.id == "cursor-fixture" and stream.result == "gator-fixture" and #stream.events == 6,
	"Cursor stream parser must preserve documented session and completion records"
)

local supported = { available = true, modes = { "native" } }
local contract = adapters.capabilities.new({
	provider = "cursor",
	transport = supported,
	auth = { available = false, reason = "Cursor status is not machine-readable" },
	session = { available = true, modes = { "create", "resume" } },
	permission = { available = true, modes = { "print" } },
	model = supported,
	command = supported,
	tool = supported,
	context = { available = false, reason = "Cursor has no documented context attachment API" },
	usage = supported,
})
local read_only = policy.new({
	scope = "run",
	target = "cursor-run",
	rules = { write_allowed = false },
	provenance = { source = "policy", ref = "test" },
})
assert(
	vim.deep_equal(adapters.cursor_policy.map(read_only, contract).args, { "--print" }),
	"Cursor read-only policy must use print mode without --force"
)
local write = policy.new({
	scope = "run",
	target = "cursor-run",
	rules = { write_allowed = true },
	provenance = { source = "policy", ref = "test" },
})
assert(
	vim.deep_equal(adapters.cursor_policy.map(write, contract), {}),
	"Cursor writable policy must preserve native approval"
)
assert(not adapters.cursor_context.submit().available, "Cursor undocumented context attachment must remain unavailable")

local calls = {}
local function run(argv)
	table.insert(calls, argv)
	return { code = 0, stdout = fixture }
end
assert(
	adapters.cursor_sessions.create({ run = run, prompt = "summarize" }).id == "cursor-fixture",
	"Cursor create must preserve native session ids"
)
assert(
	adapters.cursor_sessions.resume({ run = run, id = "cursor-fixture", prompt = "continue" }).id == "cursor-fixture",
	"Cursor resume must preserve native session ids"
)
assert(
	calls[1][2] == "--print"
		and calls[1][3] == "--output-format"
		and calls[1][4] == "stream-json"
		and calls[2][5] == "--resume"
		and calls[2][6] == "cursor-fixture",
	"Cursor sessions must use documented print, stream-json, and resume flags"
)
assert(
	not adapters.cursor_sessions.list().available and not adapters.cursor_sessions.close().available,
	"Cursor undocumented session lifecycle operations must remain unavailable"
)
