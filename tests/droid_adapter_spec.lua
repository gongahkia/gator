local adapters = require("gator").module("adapters")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local policy = require("gator").module("policy").overlay
assert(
	vim.deep_equal(adapters.droid_sessions.command({ cwd = "/tmp" }), {
		"droid",
		"exec",
		"--cwd",
		"/tmp",
		"--input-format",
		"stream-jsonrpc",
		"--output-format",
		"stream-jsonrpc",
	}),
	"Droid sessions must use a documented persistent JSON-RPC subprocess"
)
assert(vim.deep_equal(adapters.droid_sessions.create({ cwd = "/tmp" }), {
	method = "droid.initialize_session",
	params = { machineId = "gator", cwd = "/tmp", autonomyLevel = "off" },
}) and vim.deep_equal(adapters.droid_sessions.resume({ id = "droid-fixture" }), {
	method = "droid.load_session",
	params = { sessionId = "droid-fixture" },
}) and vim.deep_equal(adapters.droid_sessions.prompt({ text = "resume" }), {
	method = "droid.add_user_message",
	params = { text = "resume" },
}) and vim.deep_equal(adapters.droid_sessions.close_session(), {
	method = "droid.close_session",
	params = { reason = "other" },
}), "Droid session control must use explicit initialize, load, prompt, and close requests")
assert(
	not adapters.droid_sessions.list().available and not adapters.droid_sessions.close().available,
	"Droid undocumented list and deletion must remain unavailable"
)

local rpc = adapters.droid_stream.parse_jsonrpc(
	helpers.read(vim.g.gator_test.root .. "/tests/fixtures/adapters/droid_rpc.jsonl")
)
assert(
	rpc[1].kind == "notification"
		and rpc[1].method == "droid.session_notification"
		and rpc[1].params.notification.type == "assistant_text_delta"
		and rpc[2].kind == "response"
		and rpc[3].kind == "request"
		and rpc[3].method == "droid.request_permission",
	"Droid JSON-RPC stream must preserve notifications, responses, and permission requests"
)
assert(
	not pcall(adapters.droid_stream.parse_jsonrpc_line, [[{"jsonrpc":"2.0","id":1,"method":"droid.session_notification","params":{"notification":{"type":"assistant_text_delta"}}}]])
		and not pcall(adapters.droid_stream.parse_jsonrpc_line, [[{"jsonrpc":"2.0","id":true,"result":{}}]]),
	"Droid JSON-RPC validation must reject malformed notification and response envelopes"
)

local supported = { available = true, modes = { "native" } }
local contract = adapters.capabilities.new({
	provider = "droid",
	transport = supported,
	auth = { available = false, reason = "provider-owned" },
	session = supported,
	permission = { available = true, modes = { "user_decision" } },
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
	vim.deep_equal(adapters.droid_policy.map(read_only, contract), { initialization = { autonomyLevel = "off" } })
		and vim.deep_equal(
			adapters.droid_policy.map(writable, contract),
			{ initialization = { autonomyLevel = "off" } }
		),
	"Droid policy must retain Gator-owned per-action approval at every write level"
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
