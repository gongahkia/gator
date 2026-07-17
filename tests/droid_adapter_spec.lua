local adapters = require("gator").module("adapters")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local policy = require("gator").module("policy").overlay
local result = helpers.read(vim.g.gator_test.root .. "/tests/fixtures/adapters/droid_result.json")

local calls = {}
local function run(argv)
	table.insert(calls, argv)
	return { code = 0, stdout = result }
end
assert(
	adapters.droid_sessions.create({ run = run, cwd = "/tmp", prompt = "create" }).id == "droid-fixture",
	"Droid create must preserve the native session id"
)
assert(
	adapters.droid_sessions.resume({ run = run, id = "droid-fixture", prompt = "resume" }).id == "droid-fixture",
	"Droid resume must preserve the native session id"
)
assert(
	adapters.droid_sessions.fork({ run = run, id = "droid-fixture", prompt = "fork" }).id == "droid-fixture",
	"Droid fork must preserve the native session id"
)
assert(
	vim.deep_equal(calls[1], { "droid", "exec", "--cwd", "/tmp", "--output-format", "json", "create" })
		and vim.deep_equal(
			calls[2],
			{ "droid", "exec", "--session-id", "droid-fixture", "--output-format", "json", "resume" }
		)
		and vim.deep_equal(calls[3], { "droid", "exec", "--fork", "droid-fixture", "--output-format", "json", "fork" }),
	"Droid sessions must use only documented headless commands"
)
assert(
	not adapters.droid_sessions.list().available and not adapters.droid_sessions.close().available,
	"Droid undocumented list and deletion must remain unavailable"
)

local stream = adapters.droid_stream.parse(result)
assert(
	stream.id == "droid-fixture" and stream.text == "gator-fixture" and stream.num_turns == 1 and not stream.is_error,
	"Droid structured output must preserve documented result fields"
)
assert(not pcall(adapters.droid_stream.parse, "{}"), "Droid stream parser must reject incomplete results")
local rpc = adapters.droid_stream.parse_jsonrpc(
	helpers.read(vim.g.gator_test.root .. "/tests/fixtures/adapters/droid_rpc.jsonl")
)
assert(
	rpc[1].kind == "notification"
		and rpc[1].method == "droid.session_notification"
		and rpc[1].params.type == "assistant_text_delta"
		and rpc[2].kind == "response"
		and rpc[3].kind == "request"
		and rpc[3].method == "droid.request_permission",
	"Droid JSON-RPC stream must preserve notifications, responses, and permission requests"
)

local supported = { available = true, modes = { "native" } }
local contract = adapters.capabilities.new({
	provider = "droid",
	transport = supported,
	auth = { available = false, reason = "provider-owned" },
	session = supported,
	permission = { available = true, modes = { "autonomy" } },
	model = supported,
	command = supported,
	tool = supported,
	context = { available = false, reason = "provider-owned" },
	usage = supported,
})
local read_only = policy.new({
	scope = "run",
	target = "droid-run",
	rules = { write_allowed = false },
	provenance = { source = "policy", ref = "test" },
})
local writable = policy.new({
	scope = "run",
	target = "droid-run",
	rules = { write_allowed = true },
	provenance = { source = "policy", ref = "test" },
})
assert(
	vim.deep_equal(adapters.droid_policy.map(read_only, contract), {}),
	"Droid read-only policy must preserve default safety"
)
assert(
	vim.deep_equal(adapters.droid_policy.map(writable, contract).args, { "--auto", "low" }),
	"Droid writable policy must use the documented low autonomy level"
)

local launched
assert(adapters.droid.launch({
	manager = {
		launch = function(_, opts)
			launched = opts
			return { state = "running" }
		end,
	},
	id = "droid-run",
	cwd = vim.g.gator_test.root,
}).state == "running", "Droid launch must use shared lifecycle management")
assert(
	vim.deep_equal(launched.command, {
		"droid",
		"exec",
		"--input-format",
		"stream-jsonrpc",
		"--output-format",
		"stream-jsonrpc",
		"--cwd",
		vim.uv.fs_realpath(vim.g.gator_test.root),
	}),
	"Droid launch must use documented JSON-RPC transport"
)
assert(not adapters.droid.auth().authenticated, "Droid authentication status must remain explicit when undocumented")
assert(not pcall(adapters.droid.auth, { token = "secret" }), "Droid auth must reject credential injection")
