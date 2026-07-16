local aider = require("gator").module("adapters").aider
local fixtures = require("gator").module("adapters").fixtures
local chunks = {}
fixtures.replay_process(vim.g.gator_test.root .. "/tests/fixtures/adapters/aider_process.json", {
	stdout = function(chunk)
		table.insert(chunks, chunk)
	end,
})
local value = aider.probe({
	run = function(argv)
		return { code = 0, stdout = argv[2] == "--version" and chunks[1] or chunks[2] }
	end,
})
assert(
	value.available and value.capabilities.ask_mode and value.capabilities.architect and not value.capabilities.acp,
	"Aider probe must report documented modes without fabricating an ACP transport"
)
assert(not aider.probe({
	run = function()
		return { code = 127, stdout = "" }
	end,
}).available, "missing Aider executables must fail explicitly")
