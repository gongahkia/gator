local codex = require("gator").module("adapters").codex
local calls = {}
local value = codex.probe({
	run = function(argv)
		table.insert(calls, argv)
		if argv[2] == "--version" then
			return { code = 0, stdout = "codex-cli 0.145.0" }
		end
		return { code = 0, stdout = "--listen stdio://" }
	end,
})
assert(
	value.available and value.supported and value.capabilities.rpc,
	"Codex probe must expose supported CLI and RPC capabilities"
)
assert(#calls == 2 and calls[2][2] == "app-server", "Codex probe must inspect the structured app-server surface")
local unsupported = codex.probe({
	run = function()
		return { code = 0, stdout = "codex-cli 1.0.0" }
	end,
})
assert(unsupported.available and not unsupported.supported, "out-of-range Codex versions must remain explicit")
local missing = codex.probe({
	run = function()
		return { code = 127, stdout = "" }
	end,
})
assert(not missing.available, "missing Codex executables must fail explicitly")
local authenticated = codex.auth({
	run = function(argv)
		assert(argv[2] == "login" and argv[3] == "status", "Codex auth must query native login status")
		return { code = 0, stdout = "Logged in" }
	end,
})
assert(authenticated.authenticated, "Codex auth must preserve CLI-owned login state")
local unauthenticated = codex.auth({
	run = function()
		return { code = 1, stdout = "" }
	end,
})
assert(not unauthenticated.authenticated, "Codex auth failures must remain explicit")
local launched
local manager = {
	launch = function(_, opts)
		launched = opts
		return { id = opts.id, state = "running" }
	end,
}
assert(
	codex.launch({ manager = manager, id = "codex-run", cwd = vim.g.gator_test.root, args = { "--help" } }).state
		== "running",
	"Codex launch must delegate to the process lifecycle manager"
)
assert(
	launched.command[1] == "codex" and launched.cwd == vim.uv.fs_realpath(vim.g.gator_test.root),
	"Codex launch must preserve CLI login and map cwd safely"
)
local ok = pcall(codex.launch, { manager = manager, id = "codex-run", cwd = vim.g.gator_test.root, token = "secret" })
assert(not ok, "Codex launch must reject credential fields")
