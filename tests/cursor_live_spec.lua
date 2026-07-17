if vim.env.GATOR_LIVE_CURSOR ~= "1" then
	return
end

local cursor = require("gator").module("adapters").cursor
local stream = require("gator").module("adapters").cursor_stream
local value = cursor.probe()
assert(value.available and value.supported, "protected Cursor verification requires the supported CLI")
assert(
	value.capabilities.print
		and value.capabilities.structured_output
		and value.capabilities.session_resume
		and value.capabilities.auth_status,
	"protected Cursor verification requires documented print, stream, resume, and status capabilities"
)

if vim.env.GATOR_LIVE_CURSOR_AUTH ~= "1" then
	return
end

local workspace = vim.fn.tempname()
assert(vim.fn.mkdir(workspace, "p") == 1, "authenticated Cursor verification requires a temporary workspace")
local result = vim.system({
	"cursor-agent",
	"--print",
	"--output-format",
	"stream-json",
	"Reply exactly: gator-live-e2e",
}, { cwd = workspace, text = true }):wait()
vim.fn.delete(workspace, "d")
assert(result.code == 0, "authenticated Cursor verification requires a successful print-mode run")
local parsed = stream.parse(result.stdout or "")
assert(parsed.result == "gator-live-e2e", "Cursor E2E must preserve its exact response")
