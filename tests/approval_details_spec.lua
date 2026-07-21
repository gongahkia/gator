local details = require("gator.ui").approval_details
local approval = require("gator").module("core").approval_event

local request = approval.request({
	id = "event-approval-detail",
	run_id = "run-approval-detail",
	provider = { name = "codex", session_id = "native-approval-detail" },
	sequence = 1,
	at = 1,
	request_id = "request-approval-detail",
	action = "write file",
	details = { path = "lua/gator/ui.lua", reason = "token=fixture-secret" },
})
local decisions = {}
local window = details.open({
	request = request,
	on_decide = function(value)
		table.insert(decisions, value)
	end,
})
assert(vim.api.nvim_win_is_valid(window), "approval detail opening must create a window")
local content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(
	content:find("Provider: codex · session: native-approval-detail", 1, true)
		and content:find("Request: request-approval-detail · action: write file", 1, true)
		and not content:find("fixture-secret", 1, true),
	"approval details must render redacted request details with provider-owned session references"
)
assert(details.deny() and decisions[1].decision == "denied", "denial must route an explicit provider-native decision")
assert(
	decisions[1].provider == "codex" and decisions[1].session_id == "native-approval-detail" and not details.inspect(),
	"approval decisions must preserve provider and session ownership without retaining credentials"
)

details.open({
	request = request,
	on_decide = function()
		error("token=fixture-secret")
	end,
})
assert(not details.approve() and details.inspect().state == "failed", "decision callback failures must remain visible")
content = table.concat(vim.api.nvim_buf_get_lines(0, 0, -1, false), "\n")
assert(not content:find("fixture-secret", 1, true), "approval callback failures must be redacted")
assert(details.cancel(), "failed approval panels must close without issuing a stale decision")

window = details.open({ state = "unavailable", reason = "provider does not support approvals" })
assert(
	table
		.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
		:find("State: unavailable · provider does not support approvals", 1, true),
	"approval details must expose unavailable state"
)
assert(details.cancel(), "unavailable approval panels must close explicitly")
assert(not pcall(details.open, { request = request }), "ready approval details must require a decision callback")
assert(not pcall(details.open, {
	request = approval.decision({
		id = "event-approval-decision",
		run_id = "run-approval-detail",
		provider = { name = "codex", session_id = "native-approval-detail" },
		sequence = 2,
		at = 2,
		request_id = "request-approval-detail",
		decision = "approved",
	}),
	on_decide = function() end,
}), "approval details must reject non-request events")
