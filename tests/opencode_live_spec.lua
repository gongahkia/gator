if vim.env.GATOR_LIVE_OPENCODE ~= "1" then
	return
end

local opencode = require("gator").module("adapters").opencode
local value = opencode.probe({ cwd = vim.g.gator_test.root })
assert(value.available and value.supported, "protected OpenCode verification requires the supported CLI")
assert(
	value.capabilities.acp
		and value.capabilities.stdio
		and value.capabilities.load_session
		and value.capabilities.session_list
		and value.capabilities.session_close
		and value.capabilities.session_fork
		and value.capabilities.session_resume,
	"protected OpenCode verification requires the initialized ACP session profile"
)
local auth = opencode.auth()
assert(
	type(auth.authenticated) == "boolean" and (auth.authenticated or type(auth.reason) == "string"),
	"protected OpenCode verification must expose native credential status explicitly"
)
local agents = vim.system({ "opencode", "agent", "list" }, { text = true }):wait()
local output = agents.stdout or ""
assert(agents.code == 0, "protected OpenCode verification requires native agent modes")
assert(
	output:find("build (primary)", 1, true) and output:find("plan (primary)", 1, true),
	"protected OpenCode verification requires native build and plan modes"
)

if vim.env.GATOR_LIVE_OPENCODE_AUTH ~= "1" then
	return
end

assert(auth.authenticated, "authenticated OpenCode verification requires configured native credentials")
local workspace = vim.fn.tempname()
assert(vim.fn.mkdir(workspace, "p") == 1, "authenticated OpenCode verification requires a temporary workspace")
local result = vim.system({
	"opencode",
	"run",
	"--agent",
	"plan",
	"--format",
	"json",
	"Reply exactly: gator-live-e2e",
}, { cwd = workspace, text = true }):wait()
local session_id
local text
for line in vim.gsplit(result.stdout or "", "\n", { plain = true, trimempty = true }) do
	local ok, event = pcall(vim.json.decode, line)
	if ok and type(event) == "table" then
		if type(event.sessionID) == "string" and event.sessionID ~= "" then
			session_id = event.sessionID
		end
		if event.type == "text" and type(event.part) == "table" and type(event.part.text) == "string" then
			text = event.part.text
		end
	end
end
local recovered
local recovered_id
local recovered_text
if session_id then
	recovered = vim.system({
		"opencode",
		"run",
		"--session",
		session_id,
		"--agent",
		"plan",
		"--format",
		"json",
		"Reply exactly: gator-live-recovery",
	}, { cwd = workspace, text = true }):wait()
	for line in vim.gsplit(recovered.stdout or "", "\n", { plain = true, trimempty = true }) do
		local ok, event = pcall(vim.json.decode, line)
		if ok and type(event) == "table" then
			if type(event.sessionID) == "string" and event.sessionID ~= "" then
				recovered_id = event.sessionID
			end
			if event.type == "text" and type(event.part) == "table" and type(event.part.text) == "string" then
				recovered_text = event.part.text
			end
		end
	end
end
local deleted
if session_id then
	deleted = vim.system({ "opencode", "session", "delete", session_id }, { cwd = workspace, text = true }):wait()
end
vim.fn.delete(workspace, "d")
assert(result.code == 0, "authenticated OpenCode verification requires a successful headless run")
assert(session_id, "authenticated OpenCode verification requires a native session id")
assert(vim.trim(text or "") == "gator-live-e2e", "OpenCode E2E must preserve its exact response")
assert(
	recovered and recovered.code == 0,
	"authenticated OpenCode verification requires successful same-session recovery"
)
assert(recovered_id == session_id, "OpenCode recovery E2E must preserve the provider session id")
assert(
	vim.trim(recovered_text or "") == "gator-live-recovery",
	"OpenCode recovery E2E must preserve its exact response"
)
assert(deleted and deleted.code == 0, "OpenCode E2E must delete its native test session")
