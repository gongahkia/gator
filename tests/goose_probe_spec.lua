local goose = require("gator").module("adapters").goose
local fixtures = require("gator").module("adapters").fixtures
local chunks = {}
fixtures.replay_process(vim.g.gator_test.root .. "/tests/fixtures/adapters/goose_process.json", {
	stdout = function(chunk)
		table.insert(chunks, chunk)
	end,
})
local value = goose.probe({
	run = function(argv)
		return { code = 0, stdout = argv[2] == "--version" and chunks[1] or chunks[2] }
	end,
})
assert(
	value.available and value.capabilities.acp and value.capabilities.extensions,
	"Goose probe must expose documented ACP and extension capabilities"
)
assert(not goose.probe({
	run = function()
		return { code = 127, stdout = "" }
	end,
}).available, "missing Goose executables must fail explicitly")
