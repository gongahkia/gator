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
		and value.capabilities.session_fork
		and value.capabilities.session_resume,
	"protected OpenCode verification requires the initialized ACP session profile"
)
local auth = opencode.auth()
assert(
	type(auth.authenticated) == "boolean" and (auth.authenticated or type(auth.reason) == "string"),
	"protected OpenCode verification must expose native credential status explicitly"
)
local agents = vim.system({ "opencode", "agent", "list" }, { text = true }):wait()
local output = agents.stdout or ""
assert(agents.code == 0, "protected OpenCode verification requires native agent modes")
assert(
	output:find("build (primary)", 1, true) and output:find("plan (primary)", 1, true),
	"protected OpenCode verification requires native build and plan modes"
)
