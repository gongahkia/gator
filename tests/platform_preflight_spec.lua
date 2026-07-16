local platform = require("gator").module("performance").platform
local value = platform.inspect({
	uname = { sysname = "Linux", release = "5.15.90.1-microsoft-standard-WSL2" },
	cwd = vim.g.gator_test.root,
	executable = function(name)
		return name == "git"
	end,
	writable = function()
		return true
	end,
	run = function()
		return { code = 0, stdout = "true\n" }
	end,
})
local checks = {}
for _, check in ipairs(value.checks) do
	checks[check.name] = check
end
assert(
	value.platform == "wsl"
		and checks.platform.available
		and checks["executable.git"].available
		and not checks["executable.gator-index"].available
		and checks.filesystem.available
		and checks.worktree.available,
	"platform preflight must report WSL executables, filesystem, worktree, and sidecar readiness"
)
assert(
	not pcall(platform.inspect, { uname = { sysname = "Darwin" }, executable = true }),
	"invalid platform probes must fail explicitly"
)
