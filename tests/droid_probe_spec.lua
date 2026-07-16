local droid = require("gator").module("adapters").droid
local fixtures = require("gator").module("adapters").fixtures
local chunks = {}
fixtures.replay_process(vim.g.gator_test.root .. "/tests/fixtures/adapters/droid_process.json", {
	stdout = function(chunk)
		table.insert(chunks, chunk)
	end,
})
local value = droid.probe({
	run = function(argv)
		return { code = 0, stdout = argv[2] == "--version" and chunks[1] or chunks[2] }
	end,
})
assert(
	value.available
		and value.capabilities.execute
		and value.capabilities.structured_output
		and value.capabilities.stream_jsonrpc,
	"Droid probe must expose documented exec and stream JSON-RPC capabilities"
)
assert(not droid.probe({
	run = function()
		return { code = 127, stdout = "" }
	end,
}).available, "missing Droid executables must fail explicitly")
