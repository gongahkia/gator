local permission = require("gator").module("adapters").opencode_permission
local responses = {}
local value = permission.new({
	event_id = function(context)
		return "opencode-permission-event-" .. context.sequence
	end,
	now = function()
		return 7
	end,
	respond = function(response)
		table.insert(responses, response)
	end,
})

local function request(id, session_id)
	return {
		id = id,
		method = "session/request_permission",
		params = {
			sessionId = session_id or "opencode-fixture",
			toolCall = { toolCallId = "tool-" .. tostring(id), kind = "edit", rawInput = { token = "fixture-secret" } },
			options = {
				{ optionId = "allow-always", name = "Always allow", kind = "allow_always" },
				{ optionId = "allow-once", name = "Allow once", kind = "allow_once" },
				{ optionId = "reject-once", name = "Deny", kind = "reject_once" },
			},
		},
	}
end

local first = value:receive(request("permission-one"), { run_id = "run-opencode", session_id = "opencode-fixture" })
assert(
	first.type == "permission.request"
		and first.payload.action == "approve edit tool operation"
		and first.payload.details.tool_call_id == "tool-permission-one"
		and first.payload.details.rawInput == nil,
	"OpenCode permission requests must become explicit Gator decisions without retaining tool input"
)
local approved = value:decide(first.payload.request_id, "approved")
assert(
	approved.type == "permission.decision"
		and approved.payload.decision == "approved"
		and vim.deep_equal(responses[1], {
			id = "permission-one",
			result = { outcome = { outcome = "selected", optionId = "allow-once" } },
		}),
	"OpenCode approvals must select the least-persistent compatible native option"
)

local second = value:receive(request(2), { run_id = "run-opencode", session_id = "opencode-fixture" })
local denied = value:decide(second.payload.request_id, "denied")
assert(
	denied.payload.decision == "denied"
		and vim.deep_equal(
			responses[2],
			{ id = 2, result = { outcome = { outcome = "selected", optionId = "reject-once" } } }
		),
	"OpenCode denials must select the provided native reject option"
)

local pending = value:receive(request("permission-three"), { run_id = "run-opencode", session_id = "opencode-fixture" })
assert(value:cancel("token=fixture-secret"), "OpenCode permission cancellation must settle pending native requests")
assert(
	vim.deep_equal(responses[3], { id = "permission-three", result = { outcome = { outcome = "cancelled" } } })
		and value:status().state == "cancelled"
		and value:status().reason == "token=[REDACTED]"
		and value:status().pending == 0
		and pending.payload.request_id ~= nil,
	"OpenCode permission cancellation must send ACP cancellation and clear pending requests"
)

local mismatch = permission.new({ respond = function() end })
assert(not pcall(mismatch.receive, mismatch, request("permission-one", "other-session"), {
	run_id = "run-opencode",
	session_id = "opencode-fixture",
}), "OpenCode approvals must reject cross-session requests")

local failed = permission.new({
	respond = function()
		error("token=fixture-secret", 0)
	end,
})
local failed_request =
	failed:receive(request("permission-one"), { run_id = "run-opencode", session_id = "opencode-fixture" })
assert(
	failed:decide(failed_request.payload.request_id, "approved").state == "failed"
		and failed:status().reason == "OpenCode permission response failed",
	"OpenCode permission response failures must remain explicit and redacted"
)

local incompatible_responses = {}
local incompatible = permission.new({
	respond = function(response)
		table.insert(incompatible_responses, response)
	end,
})
local incomplete = request("permission-incomplete")
incomplete.params.options = { { optionId = "allow-once", name = "Allow once", kind = "allow_once" } }
local incomplete_request =
	incompatible:receive(incomplete, { run_id = "run-opencode", session_id = "opencode-fixture" })
assert(
	incompatible:decide(incomplete_request.payload.request_id, "denied").state == "failed"
		and incompatible:status().reason == "OpenCode permission request cannot represent denied"
		and incompatible_responses[1].result.outcome.outcome == "cancelled",
	"OpenCode permission bridges must fail closed when native options cannot represent a Gator decision"
)

local unavailable = permission.unavailable("token=fixture-secret")
assert(
	unavailable:status().state == "unavailable" and unavailable:status().reason == "token=[REDACTED]",
	"OpenCode permission unavailability must remain explicit and redacted"
)
