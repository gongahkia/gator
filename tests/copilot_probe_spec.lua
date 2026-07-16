local copilot = require("gator").module("adapters").copilot
local value = copilot.probe({
	run = function(argv)
		if argv[2] == "version" then
			return { code = 0, stdout = "GitHub Copilot CLI 0.0.411" }
		end
		return { code = 0, stdout = "--acp --resume" }
	end,
})
assert(
	value.available
		and value.supported
		and value.capabilities.acp
		and value.capabilities.stdio
		and value.capabilities.resume,
	"Copilot probe must expose the ACP stdio profile"
)
local missing = copilot.probe({
	run = function()
		return { code = 127, stdout = "" }
	end,
})
assert(not missing.available, "missing Copilot CLI must fail explicitly")
local unsupported = copilot.probe({
	run = function(argv)
		if argv[2] == "version" then
			return { code = 0, stdout = "GitHub Copilot CLI 1.0.71" }
		end
		return { code = 0, stdout = "--acp" }
	end,
})
assert(
	unsupported.available and not unsupported.supported,
	"unverified Copilot versions must not be treated as supported"
)
local launched
local manager = {
	launch = function(_, opts)
		launched = opts
		return { state = "running" }
	end,
}
assert(
	copilot.launch({ manager = manager, id = "copilot-run", cwd = vim.g.gator_test.root, args = { "--acp" } }).state
		== "running",
	"Copilot launch must use shared lifecycle management"
)
assert(
	launched.command[1] == "copilot"
		and launched.command[2] == "--acp"
		and launched.cwd == vim.uv.fs_realpath(vim.g.gator_test.root),
	"Copilot launch must preserve CLI login and safely map cwd"
)
assert(not pcall(copilot.launch, {
	manager = manager,
	id = "copilot-run",
	cwd = vim.g.gator_test.root,
	token = "secret",
}), "Copilot launch must reject credential fields")
