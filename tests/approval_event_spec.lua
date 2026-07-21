local approval = require("gator").module("core").approval_event

local request = approval.request({
	id = "event-approval-request",
	run_id = "run-approval",
	provider = { name = "codex", session_id = "native-approval" },
	sequence = 0,
	at = 1,
	request_id = "approval-one",
	action = "write file",
	details = { text = "token: private-value" },
})
local decision = approval.decision({
	id = "event-approval-decision",
	run_id = "run-approval",
	provider = { name = "codex", session_id = "native-approval" },
	sequence = 1,
	at = 2,
	request_id = "approval-one",
	decision = "approved",
})
assert(
	request.type == "permission.request"
		and request.payload.details.text:find("private%-value") == nil
		and decision.type == "permission.decision"
		and decision.payload.decision == "approved",
	"approval events must preserve native ownership while redacting durable request details"
)
assert(not pcall(approval.decision, {
	id = "event-approval-invalid",
	run_id = "run-approval",
	provider = { name = "codex" },
	sequence = 2,
	at = 3,
	request_id = "approval-one",
	decision = "unknown",
}), "approval events must reject unavailable decisions")
