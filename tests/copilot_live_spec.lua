if vim.env.GATOR_LIVE_COPILOT ~= "1" then
	return
end

local version = vim.system({ "copilot", "version" }, { text = true }):wait()
assert(
	version.code == 0 and (version.stdout or ""):find("GitHub Copilot CLI", 1, true),
	"protected Copilot verification requires the CLI version command"
)
local help = vim.system({ "copilot", "--help" }, { text = true }):wait()
local output = help.stdout or ""
assert(help.code == 0, "protected Copilot verification requires the CLI")
assert(output:find("--acp", 1, true), "protected Copilot verification requires ACP support")
assert(output:find("--resume", 1, true), "protected Copilot verification requires native session resume")
assert(output:find("--available-tools", 1, true), "protected Copilot verification requires native tool filters")

if vim.env.GATOR_LIVE_COPILOT_AUTH ~= "1" then
	return
end

local workspace = vim.fn.tempname()
assert(vim.fn.mkdir(workspace, "p") == 1, "authenticated Copilot verification requires a temporary workspace")
local result = vim.system({
	"copilot",
	"--prompt",
	"Reply exactly: gator-live-e2e",
	"--available-tools",
	"view",
	"glob",
	"grep",
	"--no-custom-instructions",
	"--silent",
	"--stream",
	"off",
}, { cwd = workspace, text = true }):wait()
vim.fn.delete(workspace, "d")
assert(result.code == 0, "authenticated Copilot verification requires a successful headless run")
assert(vim.trim(result.stdout or "") == "gator-live-e2e", "Copilot E2E must preserve its exact response")
