if vim.env.GATOR_LIVE_PI ~= "1" then
	return
end

local pi = require("gator").module("adapters").pi
local value = pi.probe({ cwd = vim.g.gator_test.root })
assert(value.available and value.supported, "protected Pi verification requires the supported CLI")
assert(
	value.capabilities.rpc
		and value.capabilities.stdio
		and value.capabilities.state
		and value.capabilities.session_create
		and value.capabilities.session_resume
		and not value.capabilities.session_list
		and not value.capabilities.session_close
		and value.capabilities.tool_filters,
	"protected Pi verification requires the credential-free RPC profile"
)
local auth = pi.auth()
assert(
	not auth.authenticated and type(auth.reason) == "string",
	"protected Pi verification must expose unavailable credential status explicitly"
)
local adapters = require("gator").module("adapters")
local overlay = require("gator").module("policy").overlay
local supported = { available = true, modes = { "native" } }
local contract = adapters.capabilities.new({
	provider = "pi",
	transport = supported,
	auth = supported,
	session = supported,
	permission = { available = true, modes = { "tool_filter" } },
	model = supported,
	command = supported,
	tool = supported,
	context = supported,
	usage = supported,
})
local read_only = overlay.new({
	scope = "run",
	target = "pi-live",
	rules = { write_allowed = false },
	provenance = { source = "protected-live", ref = "pi --help" },
})
assert(
	vim.deep_equal(adapters.pi_policy.map(read_only, contract).tools, { "read", "grep", "find", "ls" }),
	"protected Pi verification must retain the documented read-only tool allowlist"
)

if vim.env.GATOR_LIVE_PI_AUTH ~= "1" then
	return
end

local workspace = vim.fn.tempname()
assert(vim.fn.mkdir(workspace, "p") == 1, "authenticated Pi verification requires a temporary workspace")
local result = vim.system({
	"pi",
	"--mode",
	"json",
	"--no-session",
	"--no-tools",
	"--no-extensions",
	"--no-skills",
	"--no-prompt-templates",
	"--no-context-files",
	"--offline",
	"Reply exactly: gator-live-e2e",
}, { cwd = workspace, text = true }):wait()
vim.fn.delete(workspace, "d")
local text
for line in vim.gsplit(result.stdout or "", "\n", { plain = true, trimempty = true }) do
	local ok, event = pcall(vim.json.decode, line)
	if ok and type(event) == "table" and event.type == "message_end" and type(event.message) == "table" then
		local message = event.message
		if message.role == "assistant" and type(message.content) == "table" and vim.islist(message.content) then
			local parts = {}
			for _, content in ipairs(message.content) do
				if type(content) == "table" and content.type == "text" and type(content.text) == "string" then
					table.insert(parts, content.text)
				end
			end
			text = table.concat(parts)
		end
	end
end
assert(result.code == 0, "authenticated Pi verification requires a successful JSON-mode run")
assert(vim.trim(text or "") == "gator-live-e2e", "Pi E2E must preserve its exact response")
