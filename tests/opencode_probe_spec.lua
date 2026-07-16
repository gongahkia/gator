local opencode = require("gator").module("adapters").opencode
local calls = {}
local value = opencode.probe({
	cwd = vim.g.gator_test.root,
	run = function(argv, input)
		table.insert(calls, { argv = argv, input = input })
		if argv[2] == "--version" then
			return { code = 0, stdout = "1.17.15" }
		end
		return {
			code = 0,
			stdout = [[{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"agentCapabilities":{"loadSession":true,"mcpCapabilities":{"http":true,"sse":true},"promptCapabilities":{"embeddedContext":true,"image":true},"sessionCapabilities":{"close":{},"fork":{},"list":{},"resume":{}}}}}]],
		}
	end,
})
assert(
	value.available
		and value.supported
		and value.capabilities.acp
		and value.capabilities.stdio
		and value.capabilities.load_session
		and value.capabilities.session_list
		and value.capabilities.session_close
		and value.capabilities.session_fork
		and value.capabilities.mcp_http
		and value.capabilities.mcp_sse,
	"OpenCode probe must expose the initialized ACP capability profile"
)
assert(
	#calls == 2 and calls[2].argv[2] == "acp" and calls[2].input:find('"initialize"', 1, true),
	"OpenCode probe must request an ACP initialization profile"
)
local unavailable = opencode.probe({
	cwd = vim.g.gator_test.root,
	run = function()
		return { code = 127, stdout = "" }
	end,
})
assert(not unavailable.available, "missing OpenCode CLI must fail explicitly")
local unsupported = opencode.probe({
	cwd = vim.g.gator_test.root,
	run = function(argv)
		if argv[2] == "--version" then
			return { code = 0, stdout = "1.18.2" }
		end
		return { code = 1, stdout = "" }
	end,
})
assert(
	unsupported.available
		and not unsupported.supported
		and not unsupported.capabilities.acp
		and unsupported.capability_error,
	"unverified OpenCode versions and unavailable ACP profiles must remain explicit"
)
local authenticated = opencode.auth({
	run = function(argv)
		assert(argv[2] == "providers" and argv[3] == "list", "OpenCode authentication must query native credentials")
		return { code = 0, stdout = "\27[90m2 credentials\27[0m" }
	end,
})
assert(authenticated.authenticated, "OpenCode authentication must preserve CLI-owned login state")
local unauthenticated = opencode.auth({
	run = function()
		return { code = 0, stdout = "0 credentials" }
	end,
})
assert(
	not unauthenticated.authenticated and unauthenticated.reason == "OpenCode has no configured provider credentials",
	"OpenCode must report missing provider credentials explicitly"
)
local launched
local manager = {
	launch = function(_, opts)
		launched = opts
		return { state = "running" }
	end,
}
assert(
	opencode.launch({ manager = manager, id = "opencode-run", cwd = vim.g.gator_test.root }).state == "running",
	"OpenCode launch must use shared lifecycle management"
)
assert(
	launched.command[1] == "opencode"
		and launched.command[2] == "acp"
		and launched.command[3] == "--cwd"
		and launched.command[4] == vim.uv.fs_realpath(vim.g.gator_test.root)
		and launched.cwd == vim.uv.fs_realpath(vim.g.gator_test.root),
	"OpenCode launch must preserve native login and map cwd safely"
)
assert(not pcall(opencode.launch, {
	manager = manager,
	id = "opencode-run",
	cwd = vim.g.gator_test.root,
	token = "secret",
}), "OpenCode launch must reject credential fields")
