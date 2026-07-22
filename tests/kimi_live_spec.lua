if vim.env.GATOR_LIVE_KIMI ~= "1" then
	return
end

local kimi = require("gator").module("adapters").kimi
local value = kimi.probe()
assert(value.available and value.supported, "protected Kimi verification requires initialized ACP")
assert(
	value.capabilities.acp
		and value.capabilities.stdio
		and value.capabilities.load_session
		and value.capabilities.session_list
		and value.capabilities.mcp_http
		and value.capabilities.mcp_sse,
	"protected Kimi verification requires advertised ACP capabilities"
)

if vim.env.GATOR_LIVE_KIMI_AUTH ~= "1" then
	return
end

local workspace = vim.fn.tempname()
assert(vim.fn.mkdir(workspace, "p") == 1, "authenticated Kimi verification requires a temporary workspace")
local result = vim.system({
	"kimi",
	"--print",
	"--output-format",
	"text",
	"--final-message-only",
	"--plan",
	"--prompt",
	"Reply exactly: gator-live-e2e. Do not use tools or edit files.",
}, { cwd = workspace, text = true, timeout = 60000 }):wait()
vim.fn.delete(workspace, "d")
assert(result.code == 0, "authenticated Kimi verification requires a successful read-only run")
assert(
	vim.trim(result.stdout or ""):find("gator-live-e2e", 1, true),
	"authenticated Kimi verification must preserve the handoff marker"
)
