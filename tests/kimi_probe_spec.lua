local kimi = require("gator").module("adapters").kimi
local fixtures = require("gator").module("adapters").fixtures
local chunks = {}
fixtures.replay_process(vim.g.gator_test.root .. "/tests/fixtures/adapters/kimi_process.json", {
	stdout = function(chunk)
		table.insert(chunks, chunk)
	end,
})
local value = kimi.probe({
	run = function(argv)
		return { code = 0, stdout = argv[2] == "--version" and chunks[1] or chunks[2] }
	end,
})
assert(
	value.available and value.capabilities.acp and value.capabilities.session_resume and value.capabilities.print,
	"Kimi probe must expose documented ACP, session, and print capabilities"
)
assert(not kimi.probe({
	run = function()
		return { code = 127, stdout = "" }
	end,
}).available, "missing Kimi executables must fail explicitly")
