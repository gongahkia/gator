if vim.env.GATOR_LIVE_AIDER ~= "1" then
	return
end

local aider = require("gator").module("adapters").aider
local value = aider.probe()
assert(value.available and value.supported, "protected Aider verification requires the supported CLI")
assert(
	value.capabilities.message and value.capabilities.stream and value.capabilities.ask_mode,
	"protected Aider verification requires documented message, stream, and ask capabilities"
)

if vim.env.GATOR_LIVE_AIDER_AUTH ~= "1" then
	return
end

local workspace = vim.fn.tempname()
assert(vim.fn.mkdir(workspace, "p") == 1, "authenticated Aider verification requires a temporary workspace")
local result = vim.system({
	"aider",
	"--message",
	"Reply exactly: gator-live-e2e",
	"--chat-mode",
	"ask",
	"--dry-run",
	"--no-auto-commits",
	"--no-pretty",
}, { cwd = workspace, text = true }):wait()
vim.fn.delete(workspace, "d")
assert(result.code == 0, "authenticated Aider verification requires a successful read-only run")
assert((result.stdout or "") ~= "", "authenticated Aider verification requires visible terminal output")
