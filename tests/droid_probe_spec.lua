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
		assert(argv[2] == "--version" or (argv[2] == "exec" and argv[3] == "--help"), "Droid must probe exec help")
		return { code = 0, stdout = argv[2] == "--version" and chunks[1] or chunks[2] }
	end,
})
assert(
	value.available
		and value.supported
		and value.capabilities.execute
		and value.capabilities.structured_output
		and value.capabilities.stream_jsonrpc
		and value.capabilities.session_resume
		and value.capabilities.session_fork
		and value.capabilities.autonomy
		and value.capabilities.cwd,
	"Droid probe must expose the documented exec profile"
)
assert(not droid.probe({
	run = function()
		return { code = 127, stdout = "" }
	end,
}).available, "missing Droid executables must fail explicitly")
