local kimi = require("gator").module("adapters").kimi
local fixtures = require("gator").module("adapters").fixtures
local chunks = {}
fixtures.replay_process(vim.g.gator_test.root .. "/tests/fixtures/adapters/kimi_process.json", {
	stdout = function(chunk)
		table.insert(chunks, chunk)
	end,
})
local value = kimi.probe({
	run = function(argv, input)
		if argv[2] == "--version" then
			return { code = 0, stdout = chunks[1] }
		end
		assert(argv[2] == "acp" and input:find('"initialize"', 1, true), "Kimi must initialize ACP")
		return {
			code = 0,
			stdout = [[{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"agentCapabilities":{"loadSession":true,"mcpCapabilities":{"http":true,"sse":true},"promptCapabilities":{"embeddedContext":true,"image":true},"sessionCapabilities":{"list":{}}}}}]],
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
		and value.capabilities.mcp_http
		and value.capabilities.mcp_sse
		and value.capabilities.embedded_context
		and value.capabilities.plan,
	"Kimi probe must expose initialized ACP capabilities"
)
assert(not kimi.probe({
	run = function()
		return { code = 127, stdout = "" }
	end,
}).available, "missing Kimi executables must fail explicitly")
