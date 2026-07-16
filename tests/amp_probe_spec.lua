local amp = require("gator").module("adapters").amp
local fixtures = require("gator").module("adapters").fixtures
local chunks = {}
fixtures.replay_process(vim.g.gator_test.root .. "/tests/fixtures/adapters/amp_process.json", {
	stdout = function(chunk)
		table.insert(chunks, chunk)
	end,
})
local value = amp.probe({
	run = function(argv)
		return { code = 0, stdout = argv[2] == "--version" and chunks[1] or chunks[2] }
	end,
})
assert(
	value.available and value.capabilities.execute and value.capabilities.stream_json and value.capabilities.mcp,
	"Amp probe must expose documented execute, stream JSON, and MCP capabilities"
)
assert(not amp.probe({
	run = function()
		return { code = 127, stdout = "" }
	end,
}).available, "missing Amp executables must fail explicitly")
