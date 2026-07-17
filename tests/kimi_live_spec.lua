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
