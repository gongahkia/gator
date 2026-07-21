local fixtures = require("gator").module("adapters").fixtures
local permission = require("gator").module("adapters").codex_permission
local fixture = vim.g.gator_test.root .. "/tests/fixtures/adapters/codex_permissions.jsonl"
local responses = {}
local value = permission.new({
	event_id = function(context)
		return "codex-permission-event-" .. context.sequence
	end,
	now = function()
		return 7
	end,
	respond = function(response)
		table.insert(responses, response)
	end,
})
local requests = {}
fixtures.replay_jsonl(fixture, function(record)
	local event = value:receive(record, { run_id = "run-codex", session_id = "codex-fixture" })
	requests[#requests + 1] = event
end)
assert(
	requests[1].type == "permission.request"
		and requests[1].payload.action == "execute command"
		and requests[1].payload.details.reason == "token=[REDACTED]"
		and requests[1].payload.details.command == nil
		and requests[2].payload.action == "modify files"
		and requests[3].payload.action == "grant additional permissions"
		and value:status().pending == 3,
	"Codex approval requests must become explicit Gator permission events"
)
local approved = value:decide(requests[1].payload.request_id, "approved")
assert(
	approved.type == "permission.decision"
		and approved.payload.decision == "approved"
		and responses[1].id == 7
		and responses[1].result.decision == "accept",
	"explicit Gator approval must send the native one-request command decision"
)
local denied = value:decide(requests[3].payload.request_id, "denied")
assert(
	denied.payload.decision == "denied"
		and responses[2].id == "permissions-approval"
		and vim.deep_equal(responses[2].result, { permissions = {}, scope = "turn" }),
	"denied Gator permission grants must return an empty turn-scoped native profile"
)
assert(value:cancel("token=fixture-secret"), "permission bridge cancellation must deny pending native approvals")
assert(
	responses[3].id == "file-approval"
		and responses[3].result.decision == "cancel"
		and value:status().state == "cancelled"
		and value:status().reason == "token=[REDACTED]"
		and value:status().pending == 0,
	"permission bridge cancellation must clear pending requests with redacted state"
)

local mismatch = permission.new({ respond = function() end })
assert(not pcall(mismatch.receive, mismatch, {
	id = 1,
	method = "item/fileChange/requestApproval",
	params = { threadId = "other-thread", turnId = "turn-fixture", itemId = "file-fixture", startedAtMs = 1 },
}, { run_id = "run-codex", session_id = "codex-fixture" }), "Codex approvals must reject cross-thread requests")

local failed = permission.new({
	respond = function()
		error("token=fixture-secret")
	end,
})
local request = failed:receive({
	id = 1,
	method = "item/fileChange/requestApproval",
	params = { threadId = "codex-fixture", turnId = "turn-fixture", itemId = "file-fixture", startedAtMs = 1 },
}, { run_id = "run-codex", session_id = "codex-fixture" })
assert(
	failed:decide(request.payload.request_id, "approved").state == "failed"
		and failed:status().reason == "Codex permission response failed",
	"native permission response failures must remain explicit and redacted"
)

local unavailable = permission.unavailable("token=fixture-secret")
assert(
	unavailable:status().state == "unavailable" and unavailable:status().reason == "token=[REDACTED]",
	"Codex permission unavailability must remain explicit and redacted"
)
assert(
	not pcall(
		unavailable.receive,
		unavailable,
		{ id = 1, method = "item/fileChange/requestApproval", params = {} },
		{ run_id = "run-codex" }
	),
	"unavailable permission bridges must reject native requests"
)
