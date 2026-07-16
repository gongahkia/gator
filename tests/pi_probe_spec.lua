local pi = require("gator").module("adapters").pi
local calls = {}
local value = pi.probe({
	cwd = vim.g.gator_test.root,
	run = function(argv, input)
		table.insert(calls, { argv = argv, input = input })
		if argv[2] == "--version" then
			return { code = 0, stdout = "0.80.7" }
		end
		if argv[2] == "--help" then
			return { code = 0, stdout = "--mode rpc --continue --resume --session --tools --exclude-tools" }
		end
		return {
			code = 0,
			stdout = [[{"id":"gator-probe","type":"response","command":"get_state","success":true,"data":{"sessionId":"pi-fixture","isStreaming":false}}]],
		}
	end,
})
assert(
	value.available
		and value.supported
		and value.capabilities.rpc
		and value.capabilities.stdio
		and value.capabilities.state
		and value.capabilities.session_create
		and value.capabilities.session_resume
		and not value.capabilities.session_list
		and not value.capabilities.session_close
		and value.capabilities.tool_filters,
	"Pi probe must expose the initialized RPC capability profile"
)
assert(
	#calls == 3 and calls[3].argv[2] == "--mode" and calls[3].input:find('"get_state"', 1, true),
	"Pi probe must request a credential-free RPC state profile"
)
local unavailable = pi.probe({
	cwd = vim.g.gator_test.root,
	run = function()
		return { code = 127, stdout = "" }
	end,
})
assert(not unavailable.available, "missing Pi CLI must fail explicitly")
local unsupported = pi.probe({
	cwd = vim.g.gator_test.root,
	run = function(argv)
		if argv[2] == "--version" then
			return { code = 0, stdout = "0.81.0" }
		end
		return { code = 1, stdout = "" }
	end,
})
assert(
	unsupported.available
		and not unsupported.supported
		and not unsupported.capabilities.rpc
		and unsupported.capability_error,
	"unverified Pi versions and unavailable RPC profiles must remain explicit"
)
local auth = pi.auth()
assert(
	not auth.authenticated and auth.reason == "Pi RPC does not expose a non-interactive credential-status contract",
	"Pi authentication capability absence must remain explicit"
)
local launched
local manager = {
	launch = function(_, opts)
		launched = opts
		return { state = "running" }
	end,
}
assert(
	pi.launch({ manager = manager, id = "pi-run", cwd = vim.g.gator_test.root }).state == "running",
	"Pi launch must use shared lifecycle management"
)
assert(
	launched.command[1] == "pi"
		and launched.command[2] == "--mode"
		and launched.command[3] == "rpc"
		and launched.cwd == vim.uv.fs_realpath(vim.g.gator_test.root),
	"Pi launch must preserve native login and map cwd safely"
)
assert(not pcall(pi.launch, {
	manager = manager,
	id = "pi-run",
	cwd = vim.g.gator_test.root,
	token = "secret",
}), "Pi launch must reject credential fields")
