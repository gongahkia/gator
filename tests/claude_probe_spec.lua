local claude = require("gator").module("adapters").claude
local value = claude.probe({
	run = function(argv)
		if argv[2] == "--version" then
			return { code = 0, stdout = "2.1.119 (Claude Code)" }
		end
		return { code = 0, stdout = "--output-format stream-json --resume" }
	end,
})
assert(
	value.available and value.supported and value.capabilities.structured_output and value.capabilities.resume,
	"Claude probe must expose supported structured CLI capabilities"
)
local missing = claude.probe({
	run = function()
		return { code = 127, stdout = "" }
	end,
})
assert(not missing.available, "missing Claude CLI must fail explicitly")
local unknown = claude.probe({
	run = function()
		return { code = 0, stdout = "Claude" }
	end,
})
assert(not unknown.available, "unrecognized Claude versions must fail explicitly")
local authenticated = claude.auth({
	run = function(argv)
		assert(argv[2] == "auth" and argv[3] == "status", "Claude auth must query native login status")
		return { code = 0, stdout = '{"loggedIn":true}' }
	end,
})
assert(authenticated.authenticated, "Claude auth must preserve CLI-owned login state")
local unauthenticated = claude.auth({
	run = function()
		return { code = 0, stdout = '{"loggedIn":false}' }
	end,
})
assert(not unauthenticated.authenticated, "Claude auth failures must remain explicit")
local launched
local manager = {
	launch = function(_, opts)
		launched = opts
		return { state = "running" }
	end,
}
assert(
	claude.launch({ manager = manager, id = "claude-run", cwd = vim.g.gator_test.root, args = { "--help" } }).state
		== "running",
	"Claude launch must use shared lifecycle management"
)
assert(
	launched.command[1] == "claude" and launched.cwd == vim.uv.fs_realpath(vim.g.gator_test.root),
	"Claude launch must safely map cwd"
)
local ok = pcall(claude.launch, {
	manager = manager,
	id = "claude-run",
	cwd = vim.g.gator_test.root,
	token = "secret",
})
assert(not ok, "Claude launch must reject credential fields")
