local vibe = require("gator").module("adapters").vibe
local fixtures = require("gator").module("adapters").fixtures
local chunks = {}
fixtures.replay_process(vim.g.gator_test.root .. "/tests/fixtures/adapters/vibe_process.json", {
	stdout = function(chunk)
		table.insert(chunks, chunk)
	end,
})
local value = vibe.probe({
	run = function(argv)
		return { code = 0, stdout = argv[2] == "--version" and chunks[1] or chunks[2] }
	end,
})
assert(
	value.available and value.capabilities.acp and value.capabilities.structured_output and value.capabilities.plan,
	"Vibe probe must expose documented ACP, output, plan, and approval capabilities"
)
assert(not vibe.probe({
	run = function()
		return { code = 127, stdout = "" }
	end,
}).available, "missing Vibe executables must fail explicitly")
