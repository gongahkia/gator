local capabilities = require("gator").module("adapters").capabilities
local supported = { available = true, modes = { "native" } }
local value = capabilities.new({
	provider = "codex",
	transport = { available = true, modes = { "stdio", "jsonrpc" } },
	auth = supported,
	session = supported,
	permission = { available = false, reason = "provider does not expose permission controls" },
	model = supported,
	command = supported,
	tool = supported,
	context = supported,
	usage = supported,
})

assert(capabilities.is(value), "adapter capabilities must have a distinct entity type")
assert(capabilities.supports(value, "transport", "jsonrpc"), "advertised transport modes must be supported")
local available, reason = capabilities.supports(value, "permission")
assert(not available and reason:find("does not expose", 1, true), "missing capabilities must expose their reason")
assert(
	capabilities.from_record(capabilities.to_record(value)).provider == "codex",
	"capability contracts must round-trip"
)
local ok = pcall(capabilities.require, value, "permission")
assert(not ok, "unavailable capabilities must fail explicitly")
ok = pcall(capabilities.new, { provider = "codex", transport = supported })
assert(not ok, "capability contracts must declare every required domain")
