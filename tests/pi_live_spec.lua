if vim.env.GATOR_LIVE_PI ~= "1" then
	return
end

local pi = require("gator").module("adapters").pi
local value = pi.probe({ cwd = vim.g.gator_test.root })
assert(value.available and value.supported, "protected Pi verification requires the supported CLI")
assert(
	value.capabilities.rpc
		and value.capabilities.stdio
		and value.capabilities.state
		and value.capabilities.sessions
		and value.capabilities.tool_filters,
	"protected Pi verification requires the credential-free RPC profile"
)
