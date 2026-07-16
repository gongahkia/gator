if vim.env.GATOR_LIVE_GEMINI ~= "1" then
	return
end

local help = vim.system({ "gemini", "--help" }, { text = true }):wait()
local output = help.stdout or ""
assert(help.code == 0, "protected Gemini verification requires the CLI")
assert(output:find("--list-sessions", 1, true), "protected Gemini verification requires native session listing")
assert(output:find("--delete-session", 1, true), "protected Gemini verification requires native session deletion")
assert(output:find("stream-json", 1, true), "protected Gemini verification requires native session init events")
