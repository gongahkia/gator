if vim.env.GATOR_LIVE_AMP ~= "1" then
	return
end

local amp = require("gator").module("adapters").amp
local value = amp.probe()
assert(value.available and value.supported, "protected Amp verification requires the supported CLI")
assert(
	value.capabilities.execute
		and value.capabilities.stream_json
		and value.capabilities.stream_input
		and value.capabilities.threads,
	"protected Amp verification requires documented stream and thread capabilities"
)

if vim.env.GATOR_LIVE_AMP_AUTH ~= "1" then
	return
end

local workspace = vim.fn.tempname()
assert(vim.fn.mkdir(workspace, "p") == 1, "authenticated Amp verification requires a temporary workspace")
local result = vim.system({
	"amp",
	"--execute",
	"Reply exactly: gator-live-e2e",
	"--stream-json",
	"--no-archive-after-execute",
}, { cwd = workspace, text = true }):wait()
local session_id
local complete = false
for line in vim.gsplit(result.stdout or "", "\n", { plain = true, trimempty = true }) do
	local ok, event = pcall(vim.json.decode, line)
	if ok and type(event) == "table" then
		if type(event.session_id) == "string" and event.session_id ~= "" then
			session_id = event.session_id
		end
		complete = complete or (event.type == "result" and event.subtype == "success" and event.is_error == false)
	end
end
local deleted
if session_id then
	deleted = vim.system({ "amp", "threads", "delete", session_id }, { cwd = workspace, text = true }):wait()
end
vim.fn.delete(workspace, "d")
assert(result.code == 0, "authenticated Amp verification requires a successful stream run")
assert(session_id, "authenticated Amp verification requires a provider-native thread id")
assert(complete, "authenticated Amp verification requires a successful stream result")
assert(deleted and deleted.code == 0, "Amp E2E must delete its native test thread")
