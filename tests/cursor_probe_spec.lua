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
	value.available
		and value.supported
		and value.capabilities.print
		and value.capabilities.structured_output
		and value.capabilities.session_resume
		and value.capabilities.auth_status,
	"Cursor probe must expose documented print, stream, session, and status capabilities"
)
assert(not cursor.probe({
	run = function()
		return { code = 127, stdout = "" }
	end,
}).available, "missing Cursor executables must fail explicitly")
local launched
local manager = {
	launch = function(_, opts)
		launched = opts
		return { state = "running" }
	end,
}
assert(
	cursor.launch({ manager = manager, id = "cursor-run", cwd = vim.g.gator_test.root }).state == "running",
	"Cursor launch must use shared lifecycle management"
)
assert(
	launched.command[1] == "cursor-agent" and launched.cwd == vim.uv.fs_realpath(vim.g.gator_test.root),
	"Cursor launch must preserve native login and map cwd safely"
)
assert(
	not cursor.auth().authenticated and cursor.auth().reason:find("machine-readable", 1, true),
	"Cursor must expose undocumented auth parsing explicitly"
)
assert(not pcall(cursor.launch, {
	manager = manager,
	id = "cursor-run",
	cwd = vim.g.gator_test.root,
	token = "secret",
}), "Cursor launch must reject credential fields")
