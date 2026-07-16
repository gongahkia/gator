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
