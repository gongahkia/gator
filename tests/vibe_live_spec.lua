if vim.env.GATOR_LIVE_VIBE ~= "1" then
	return
end

local vibe = require("gator").module("adapters").vibe
local value = vibe.probe()
assert(value.available and value.supported, "protected Vibe verification requires initialized ACP")
assert(
	value.capabilities.acp
		and value.capabilities.stdio
		and value.capabilities.load_session
		and value.capabilities.session_list
		and value.capabilities.session_close
		and value.capabilities.session_fork,
	"protected Vibe verification requires advertised ACP session capabilities"
)

if vim.env.GATOR_LIVE_VIBE_AUTH ~= "1" then
	return
end

local workspace = vim.fn.tempname()
assert(vim.fn.mkdir(workspace, "p") == 1, "authenticated Vibe verification requires a temporary workspace")
local result = vim.system({
	"vibe",
	"--prompt",
	"Reply exactly: gator-live-e2e. Do not use tools or edit files.",
	"--max-turns",
	"1",
	"--output",
	"text",
	"--agent",
	"plan",
	"--trust",
}, { cwd = workspace, text = true, timeout = 60000 }):wait()
vim.fn.delete(workspace, "d")
assert(result.code == 0, "authenticated Vibe verification requires a successful read-only run")
assert(
	vim.trim(result.stdout or ""):find("gator-live-e2e", 1, true),
	"authenticated Vibe verification must preserve the handoff marker"
)
