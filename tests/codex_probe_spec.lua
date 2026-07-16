local codex = require("gator").module("adapters").codex
local calls = {}
local value = codex.probe({
	run = function(argv)
		table.insert(calls, argv)
		if argv[2] == "--version" then
			return { code = 0, stdout = "codex-cli 0.144.4" }
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
