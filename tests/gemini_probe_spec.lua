local gemini = require("gator").module("adapters").gemini
local value = gemini.probe({
	run = function(argv)
		if argv[2] == "--version" then
			return { code = 0, stdout = "0.46.0" }
		end
		return { code = 0, stdout = "--acp --output-format stream-json --list-sessions --resume" }
	end,
})
assert(
	value.available
		and value.supported
		and value.capabilities.acp
		and value.capabilities.structured_output
		and value.capabilities.sessions,
	"Gemini probe must expose ACP and structured CLI capabilities"
)
local missing = gemini.probe({
	run = function()
		return { code = 127, stdout = "" }
	end,
})
assert(not missing.available, "missing Gemini CLI must fail explicitly")
local launched
local manager = {
	launch = function(_, opts)
		launched = opts
		return { state = "running" }
	end,
}
assert(
	gemini.launch({ manager = manager, id = "gemini-run", cwd = vim.g.gator_test.root, args = { "--help" } }).state
		== "running",
	"Gemini launch must use shared lifecycle management"
)
assert(
	launched.command[1] == "gemini" and launched.cwd == vim.uv.fs_realpath(vim.g.gator_test.root),
	"Gemini launch must safely map cwd"
)
local ok = pcall(gemini.launch, {
	manager = manager,
	id = "gemini-run",
	cwd = vim.g.gator_test.root,
	token = "secret",
})
assert(not ok, "Gemini launch must reject credential fields")
