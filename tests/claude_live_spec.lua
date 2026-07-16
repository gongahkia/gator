if vim.env.GATOR_LIVE_CLAUDE ~= "1" then
	return
end

local result = vim.system({ "claude", "auth", "status" }, { text = true }):wait()
assert(result.code == 0, "protected Claude verification requires an existing CLI-owned login")
local ok, status = pcall(vim.json.decode, result.stdout or "")
assert(
	ok and type(status) == "table" and status.loggedIn == true,
	"protected Claude verification must prove the CLI-owned login state"
)
local help = vim.system({ "claude", "--help" }, { text = true }):wait()
assert(
	help.code == 0 and (help.stdout or ""):find("stream-json", 1, true),
	"protected Claude verification requires structured stream support"
)
