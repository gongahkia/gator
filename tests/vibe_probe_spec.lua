local vibe = require("gator").module("adapters").vibe
local fixtures = require("gator").module("adapters").fixtures
local chunks = {}
fixtures.replay_process(vim.g.gator_test.root .. "/tests/fixtures/adapters/vibe_process.json", {
	stdout = function(chunk)
		table.insert(chunks, chunk)
	end,
})
local value = vibe.probe({
	run = function(argv, input)
		if argv[1] == "vibe" and argv[2] == "--version" then
			return { code = 0, stdout = chunks[1] }
		end
		assert(argv[1] == "vibe-acp" and input:find('"initialize"', 1, true), "Vibe must initialize vibe-acp")
		return {
			code = 0,
			stdout = [[{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"agentCapabilities":{"loadSession":true,"promptCapabilities":{"embeddedContext":true,"image":true},"sessionCapabilities":{"close":{},"fork":{},"list":{}}}}}]],
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
		and value.capabilities.embedded_context
		and value.capabilities.structured_output
		and value.capabilities.plan,
	"Vibe probe must expose initialized ACP capabilities"
)
assert(not vibe.probe({
	run = function()
		return { code = 127, stdout = "" }
	end,
}).available, "missing Vibe executables must fail explicitly")
