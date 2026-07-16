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
		and value.capabilities.session_create
		and value.capabilities.session_resume
		and not value.capabilities.session_list
		and not value.capabilities.session_close
		and value.capabilities.tool_filters,
	"protected Pi verification requires the credential-free RPC profile"
)
local auth = pi.auth()
assert(
	not auth.authenticated and type(auth.reason) == "string",
	"protected Pi verification must expose unavailable credential status explicitly"
)
