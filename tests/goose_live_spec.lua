if vim.env.GATOR_LIVE_GOOSE ~= "1" then
	return
end

local goose = require("gator").module("adapters").goose
local value = goose.probe()
assert(value.available and value.supported, "protected Goose verification requires initialized ACP")
assert(
	value.capabilities.acp
		and value.capabilities.stdio
		and value.capabilities.load_session
		and value.capabilities.session_list
		and value.capabilities.session_close,
	"protected Goose verification requires advertised ACP session capabilities"
)

if vim.env.GATOR_LIVE_GOOSE_AUTH ~= "1" then
	return
end

local workspace = vim.fn.tempname()
assert(vim.fn.mkdir(workspace, "p") == 1, "authenticated Goose verification requires a temporary workspace")
local result = vim.system({
	"goose",
	"run",
	"--no-session",
	"-t",
	"Reply exactly: gator-live-e2e",
	"--output-format",
	"json",
}, { cwd = workspace, text = true }):wait()
vim.fn.delete(workspace, "d")
assert(
	result.code == 0 and vim.trim(result.stdout or "") ~= "",
	"authenticated Goose verification requires a successful stateless run"
)
