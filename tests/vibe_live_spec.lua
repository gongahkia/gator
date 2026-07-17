if vim.env.GATOR_LIVE_VIBE ~= "1" then
	return
end

local vibe = require("gator").module("adapters").vibe
local value = vibe.probe()
assert(value.available and value.supported, "protected Vibe verification requires initialized ACP")
assert(
	value.capabilities.acp
		and value.capabilities.stdio
		and value.capabilities.load_session
		and value.capabilities.session_list
		and value.capabilities.session_close
		and value.capabilities.session_fork,
	"protected Vibe verification requires advertised ACP session capabilities"
)
