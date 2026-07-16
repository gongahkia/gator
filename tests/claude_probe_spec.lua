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
