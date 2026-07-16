if vim.env.GATOR_LIVE_CODEX ~= "1" then
	return
end

local login = vim.system({ "codex", "login", "status" }, { text = true }):wait()
assert(login.code == 0, "protected Codex verification requires an existing CLI-owned login")
local app_server = vim.system({ "codex", "app-server", "--help" }, { text = true }):wait()
assert(
	app_server.code == 0 and (app_server.stdout or ""):find("stdio://", 1, true),
	"protected Codex verification requires the app-server stdio capability"
)
