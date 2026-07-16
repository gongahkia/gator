if vim.env.GATOR_LIVE_GEMINI ~= "1" then
	return
end

local help = vim.system({ "gemini", "--help" }, { text = true }):wait()
local output = help.stdout or ""
assert(help.code == 0, "protected Gemini verification requires the CLI")
assert(output:find("--list-sessions", 1, true), "protected Gemini verification requires native session listing")
assert(output:find("--delete-session", 1, true), "protected Gemini verification requires native session deletion")
assert(output:find("stream-json", 1, true), "protected Gemini verification requires native session init events")
assert(output:find("--approval-mode", 1, true), "protected Gemini verification requires native approval modes")
assert(output:find("--prompt", 1, true), "protected Gemini verification requires headless prompt support")

if vim.env.GATOR_LIVE_GEMINI_AUTH ~= "1" then
	return
end

local workspace = vim.fn.tempname()
assert(vim.fn.mkdir(workspace, "p") == 1, "authenticated Gemini verification requires a temporary workspace")
local result = vim.system({
	"gemini",
	"--prompt",
	"Reply exactly: gator-live-e2e",
	"--approval-mode",
	"plan",
	"--output-format",
	"stream-json",
}, { cwd = workspace, text = true }):wait()
local session_id
local complete = false
for line in vim.gsplit(result.stdout or "", "\n", { plain = true, trimempty = true }) do
	local ok, event = pcall(vim.json.decode, line)
	if ok and type(event) == "table" then
		if event.type == "init" and type(event.session_id) == "string" and event.session_id ~= "" then
			session_id = event.session_id
		end
		complete = complete or (event.type == "result" and event.status == "success")
	end
end
local deleted
if session_id then
	deleted = vim.system({ "gemini", "--delete-session", session_id }, { cwd = workspace, text = true }):wait()
end
vim.fn.delete(workspace, "d")
assert(result.code == 0, "authenticated Gemini verification requires a successful headless run")
assert(session_id, "authenticated Gemini verification requires a native session id")
assert(complete, "authenticated Gemini verification requires a successful stream result")
assert(
	deleted and deleted.code == 0 and (deleted.stdout or ""):match("^Deleted session "),
	"Gemini E2E must delete its native session"
)
