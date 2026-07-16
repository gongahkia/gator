local cursor = require("gator").module("adapters").cursor
local fixtures = require("gator").module("adapters").fixtures
local chunks = {}
fixtures.replay_process(vim.g.gator_test.root .. "/tests/fixtures/adapters/cursor_process.json", {
	stdout = function(chunk)
		table.insert(chunks, chunk)
	end,
})
local value = cursor.probe({
	run = function(argv)
		return { code = 0, stdout = argv[2] == "--version" and chunks[1] or chunks[2] }
	end,
})
assert(
	value.available and value.supported and value.capabilities.structured_output and value.capabilities.session_resume,
	"Cursor probe must expose documented print, stream, and session capabilities"
)
assert(not cursor.probe({
	run = function()
		return { code = 127, stdout = "" }
	end,
}).available, "missing Cursor executables must fail explicitly")
