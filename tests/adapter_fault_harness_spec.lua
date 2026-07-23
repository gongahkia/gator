local fixtures = require("gator").module("adapters").fixtures
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local root = vim.g.gator_test.root .. "/tests/fixtures/adapters"

local received = {}
local unavailable = fixtures.replay_with_faults({
	kind = "jsonl",
	path = root .. "/stream.jsonl",
	failures = {
		{
			operation = "record",
			occurrence = 2,
			state = "unavailable",
			kind = "offline",
			reason = "provider is unavailable",
		},
	},
	on_record = function(record)
		table.insert(received, record.type)
	end,
})
assert(
	unavailable.state == "unavailable" and unavailable.count == 1 and vim.deep_equal(received, { "message" }),
	"fault replays must stop before the configured provider event and retain prior delivery"
)

local callback_failed = fixtures.replay_with_faults({
	kind = "terminal",
	path = root .. "/aider_terminal.json",
	on_record = function()
		error("token: private-value")
	end,
})
assert(
	callback_failed.state == "failed"
		and callback_failed.kind == "callback"
		and not callback_failed.reason:find("private%-value"),
	"adapter parser failures must be explicit and redacted"
)

local cancelled = fixtures.replay_with_faults({
	kind = "jsonrpc",
	path = root .. "/rpc.jsonl",
	cancel = function(index)
		return index == 2
	end,
	on_record = function() end,
})
assert(
	cancelled.state == "cancelled" and cancelled.count == 1,
	"fault replays must support deterministic cancellation between provider events"
)

local malformed = helpers.tempdir("adapter-fault-harness") .. "/malformed.jsonl"
helpers.write(
	malformed,
	'{"jsonrpc":"2.0","id":1,"result":{},"error":{"code":-32603,"message":"token: private-value"}}\n'
)
local rejected = fixtures.replay_with_faults({
	kind = "jsonrpc",
	path = malformed,
	on_record = function() end,
})
assert(
	rejected.state == "failed" and rejected.kind == "malformed_fixture" and not rejected.reason:find("private%-value"),
	"malformed provider events must return redacted deterministic failures"
)
assert(
	not pcall(
		fixtures.replay_with_faults,
		{ kind = "process", path = root .. "/process.json", on_record = function() end }
	),
	"fault replays must reject unsupported fixture transports"
)
