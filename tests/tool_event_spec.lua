local tool = require("gator").module("core").tool_event

local common = {
	id = "event-tool-call",
	run_id = "run-tool",
	provider = { name = "codex", session_id = "native-tool" },
	sequence = 0,
	at = 1,
}
local call = tool.call(vim.tbl_extend("force", common, {
	call_id = "call-tool",
	name = "read_file",
	input = { path = "README.md", text = "token: private-value" },
}))
local result = tool.result({
	id = "event-tool-result",
	run_id = "run-tool",
	provider = { name = "codex", session_id = "native-tool" },
	sequence = 1,
	at = 2,
	call_id = "call-tool",
	state = "success",
	output = { text = "token: private-value" },
})
assert(
	call.type == "tool.call"
		and call.payload.input.text:find("private%-value") == nil
		and result.type == "tool.result"
		and result.payload.state == "completed"
		and result.provider.session_id == "native-tool",
	"tool event normalization must preserve native session identity while redacting payloads"
)
assert(
	not pcall(tool.result, vim.tbl_extend("force", common, { call_id = "call-tool", state = "unknown" })),
	"tool event normalization must reject unavailable result states"
)
