local cline = require("gator").module("adapters").cline
local fixtures = require("gator").module("adapters").fixtures
local chunks = {}
fixtures.replay_process(vim.g.gator_test.root .. "/tests/fixtures/adapters/cline_process.json", {
	stdout = function(chunk)
		table.insert(chunks, chunk)
	end,
})
local value = cline.probe({
	run = function(argv)
		return { code = 0, stdout = argv[2] == "--version" and chunks[1] or chunks[2] }
	end,
})
assert(
	value.available and value.capabilities.acp and value.capabilities.structured_output and value.capabilities.plan,
	"Cline probe must expose documented ACP, JSON, session, and plan capabilities"
)
assert(not cline.probe({
	run = function()
		return { code = 127, stdout = "" }
	end,
}).available, "missing Cline executables must fail explicitly")
