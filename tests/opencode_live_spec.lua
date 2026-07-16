if vim.env.GATOR_LIVE_OPENCODE ~= "1" then
	return
end

local opencode = require("gator").module("adapters").opencode
local value = opencode.probe({ cwd = vim.g.gator_test.root })
assert(value.available and value.supported, "protected OpenCode verification requires the supported CLI")
assert(
	value.capabilities.acp
		and value.capabilities.stdio
		and value.capabilities.load_session
		and value.capabilities.session_list
		and value.capabilities.session_close
		and value.capabilities.session_fork,
	"protected OpenCode verification requires the initialized ACP session profile"
)
