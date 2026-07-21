local permission = require("gator").module("adapters").claude_permission
local responses = {}
local value = permission.new({
	event_id = function(context)
		return "claude-permission-event-" .. context.sequence
	end,
	now = function()
		return 7
	end,
	respond = function(response)
		table.insert(responses, response)
	end,
})
local command = value:receive({ tool_name = "Bash", input = { command = "token=fixture-secret" } }, {
	run_id = "run-claude",
	session_id = "claude-fixture",
})
assert(
	command.type == "permission.request"
		and command.payload.action == "execute command"
		and command.payload.details.tool == "Bash"
		and command.payload.details.command == nil,
	"Claude tool approvals must become explicit Gator decisions without retaining tool input"
)
local approved = value:decide(command.payload.request_id, "approved")
assert(
	approved.type == "permission.decision"
		and approved.payload.decision == "approved"
		and responses[1].behavior == "allow"
		and responses[1].updatedInput.command == "token=fixture-secret",
	"approved Gator decisions must pass the original input only to the native callback"
)

local write = value:receive({ tool_name = "Write", input = { file_path = "fixture" } }, {
	run_id = "run-claude",
	session_id = "claude-fixture",
})
local denied = value:decide(write.payload.request_id, "denied")
assert(denied.payload.decision == "denied" and vim.deep_equal(responses[2], {
	behavior = "deny",
	message = "Gator denied Claude tool request",
}), "denied Gator decisions must return a native denial")

local pending = value:receive({ tool_name = "Read", input = { file_path = "fixture" } }, {
	run_id = "run-claude",
	session_id = "claude-fixture",
})
assert(value:cancel("token=fixture-secret"), "Claude permission bridge cancellation must deny pending native requests")
assert(
	responses[3].behavior == "deny"
		and responses[3].message == "Gator cancelled Claude tool request"
		and value:status().state == "cancelled"
		and value:status().reason == "token=[REDACTED]"
		and value:status().pending == 0
		and pending.payload.request_id ~= nil,
	"Claude permission cancellation must clear pending requests with redacted state"
)

local invalid = permission.new({ respond = function() end })
assert(
	not pcall(invalid.receive, invalid, { tool_name = "Bash", input = { "not-an-object" } }, { run_id = "run-claude" }),
	"Claude permission bridge must reject invalid native input"
)

local failed = permission.new({
	respond = function()
		error("token=fixture-secret", 0)
	end,
})
local request = failed:receive({ tool_name = "Edit", input = { file_path = "fixture" } }, { run_id = "run-claude" })
assert(
	failed:decide(request.payload.request_id, "approved").state == "failed"
		and failed:status().reason == "Claude permission response failed",
	"Claude permission response failures must remain explicit and redacted"
)

local unavailable = permission.unavailable("token=fixture-secret")
assert(
	unavailable:status().state == "unavailable" and unavailable:status().reason == "token=[REDACTED]",
	"Claude permission unavailability must remain explicit and redacted"
)
