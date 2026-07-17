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
	value.available
		and value.capabilities.execute
		and value.capabilities.stream_json
		and value.capabilities.stream_input
		and value.capabilities.mcp,
	"Amp probe must expose documented execute, stream JSON, and MCP capabilities"
)
assert(not amp.probe({
	run = function()
		return { code = 127, stdout = "" }
	end,
}).available, "missing Amp executables must fail explicitly")
local launched
local manager = {
	launch = function(_, opts)
		launched = opts
		return { state = "running" }
	end,
}
assert(
	amp.launch({ manager = manager, id = "amp-run", cwd = vim.g.gator_test.root }).state == "running",
	"Amp launch must use shared lifecycle management"
)
assert(
	launched.command[1] == "amp" and launched.cwd == vim.uv.fs_realpath(vim.g.gator_test.root),
	"Amp launch must preserve provider-native credentials and map cwd safely"
)
assert(
	not amp.auth().authenticated and amp.auth().reason:find("non-interactive", 1, true),
	"Amp must expose unavailable authentication status explicitly"
)
assert(not pcall(amp.launch, {
	manager = manager,
	id = "amp-run",
	cwd = vim.g.gator_test.root,
	token = "secret",
}), "Amp launch must reject credential fields")
