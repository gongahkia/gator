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
local launched
local manager = {
	launch = function(_, opts)
		launched = opts
		return { state = "running" }
	end,
}
assert(
	aider.launch({ manager = manager, id = "aider-run", cwd = vim.g.gator_test.root }).state == "running",
	"Aider launch must use shared lifecycle management"
)
assert(
	launched.command[1] == "aider" and launched.cwd == vim.uv.fs_realpath(vim.g.gator_test.root),
	"Aider launch must preserve provider-native credentials and map cwd safely"
)
assert(
	not aider.auth().authenticated and aider.auth().reason:find("provider-independent", 1, true),
	"Aider must expose unavailable authentication status explicitly"
)
assert(not pcall(aider.launch, {
	manager = manager,
	id = "aider-run",
	cwd = vim.g.gator_test.root,
	token = "secret",
}), "Aider launch must reject credential fields")
