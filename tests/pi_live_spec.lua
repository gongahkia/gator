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
