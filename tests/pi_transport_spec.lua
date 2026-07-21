local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local transport = require("gator").module("adapters").pi_transport
local fixture = vim.g.gator_test.root .. "/tests/fixtures/adapters/pi_rpc.jsonl"

local records = {}
local report = transport.new():replay(fixture, function(record, meta)
	records[meta.index] = meta.kind .. ":" .. (record.command or record.type)
end)
assert(
	report.records == 5 and report.responses == 3 and report.events == 2 and report.failures == 0,
	"Pi RPC fixture must conform to the recorded transport schema"
)
assert(
	vim.deep_equal(
		records,
		{ "response:get_state", "response:get_commands", "response:prompt", "event:message_end", "event:agent_end" }
	),
	"Pi RPC fixture must preserve native response and stream event order"
)

local incomplete = helpers.tempdir("pi-transport") .. "/incomplete.jsonl"
helpers.write(
	incomplete,
	'{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"fixture"}]}}\n'
)
local incomplete_transport = transport.new()
assert(
	not pcall(incomplete_transport.replay, incomplete_transport, incomplete),
	"Pi fixtures must reject stream events without a prompt"
)

local failed = helpers.tempdir("pi-transport-error") .. "/failed.jsonl"
helpers.write(
	failed,
	'{"id":"gator-fixture","type":"response","command":"prompt","success":false,"error":"provider unavailable"}\n'
)
assert(transport.new():replay(failed).failures == 1, "Pi fixtures must classify native failure responses")

local unavailable = transport.unavailable("Pi token=fixture-secret is unavailable")
assert(
	unavailable:status().state == "unavailable" and unavailable:status().reason == "Pi token=[REDACTED] is unavailable",
	"Pi transport unavailability must be explicit and redacted"
)
assert(not pcall(unavailable.replay, unavailable, fixture), "unavailable Pi transports must reject fixture replay")

local cancelled = transport.new()
assert(cancelled:cancel("token=fixture-secret"), "ready Pi transports must support cancellation")
assert(
	cancelled:status().state == "cancelled" and cancelled:status().reason == "token=[REDACTED]",
	"Pi transport cancellation must redact reasons"
)
assert(not pcall(cancelled.replay, cancelled, fixture), "cancelled Pi transports must reject fixture replay")
