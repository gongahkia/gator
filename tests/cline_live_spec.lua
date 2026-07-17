if vim.env.GATOR_LIVE_CLINE ~= "1" then
	return
end

local cline = require("gator").module("adapters").cline
local stream = require("gator").module("adapters").cline_stream
local value = cline.probe({ cwd = vim.g.gator_test.root })
assert(value.available and value.supported, "protected Cline verification requires the supported CLI")
assert(
	value.capabilities.acp
		and value.capabilities.stdio
		and value.capabilities.structured_output
		and value.capabilities.session_create
		and value.capabilities.session_resume
		and value.capabilities.plan,
	"protected Cline verification requires initialized ACP and documented CLI capabilities"
)

if vim.env.GATOR_LIVE_CLINE_AUTH ~= "1" then
	return
end

local workspace = vim.fn.tempname()
assert(vim.fn.mkdir(workspace, "p") == 1, "authenticated Cline verification requires a temporary workspace")
local result = vim.system({
	"cline",
	"--json",
	"--plan",
	"--auto-approve",
	"false",
	"Reply exactly: gator-live-e2e",
}, { cwd = workspace, text = true }):wait()
vim.fn.delete(workspace, "d")
assert(result.code == 0, "authenticated Cline verification requires a successful plan-mode run")
assert(#stream.parse(result.stdout or "") > 0, "authenticated Cline verification requires documented JSON output")
