local goose = require("gator").module("adapters").goose
local fixtures = require("gator").module("adapters").fixtures
local chunks = {}
fixtures.replay_process(vim.g.gator_test.root .. "/tests/fixtures/adapters/goose_process.json", {
	stdout = function(chunk)
		table.insert(chunks, chunk)
	end,
})
local value = goose.probe({
	run = function(argv, input)
		if argv[2] == "--version" then
			return { code = 0, stdout = chunks[1] }
		end
		assert(argv[2] == "acp" and input:find('"initialize"', 1, true), "Goose must initialize ACP")
		return {
			code = 0,
			stdout = [[{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"agentCapabilities":{"loadSession":true,"mcpCapabilities":{"http":true},"promptCapabilities":{"embeddedContext":true,"image":true},"sessionCapabilities":{"close":{},"list":{}}}}}]],
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
		and value.capabilities.embedded_context
		and value.capabilities.extensions,
	"Goose probe must expose initialized ACP and extension capabilities"
)
assert(not goose.probe({
	run = function()
		return { code = 127, stdout = "" }
	end,
}).available, "missing Goose executables must fail explicitly")
