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
