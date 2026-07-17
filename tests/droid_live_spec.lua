if vim.env.GATOR_LIVE_DROID ~= "1" then
	return
end

local droid = require("gator").module("adapters").droid
local stream = require("gator").module("adapters").droid_stream
local value = droid.probe()
assert(value.available and value.supported, "protected Droid verification requires the documented exec profile")
assert(
	value.capabilities.structured_output
		and value.capabilities.stream_jsonrpc
		and value.capabilities.session_resume
		and value.capabilities.session_fork
		and value.capabilities.autonomy,
	"protected Droid verification requires documented execution capabilities"
)

if vim.env.GATOR_LIVE_DROID_AUTH ~= "1" then
	return
end

local workspace = vim.fn.tempname()
assert(vim.fn.mkdir(workspace, "p") == 1, "authenticated Droid verification requires a temporary workspace")
local result = vim.system({
	"droid",
	"exec",
	"--cwd",
	workspace,
	"--output-format",
	"json",
	"Reply with a brief greeting.",
}, { cwd = workspace, text = true }):wait()
vim.fn.delete(workspace, "d")
assert(result.code == 0, "authenticated Droid verification requires a successful read-only run")
assert(
	stream.parse(result.stdout or "").text ~= "",
	"authenticated Droid verification requires a documented JSON result"
)
