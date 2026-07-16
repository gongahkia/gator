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
	target = "run-one",
	rules = { write_allowed = false },
	provenance = { source = "policy", ref = "test" },
})
assert(
	vim.deep_equal(adapters.pi_policy.map(read_only, contract).tools, { "read", "grep", "find", "ls" }),
	"read-only Pi policy must use the native read-only tool allowlist"
)
local write = overlay.new({
	scope = "run",
	target = "run-one",
	rules = { write_allowed = true },
	provenance = { source = "policy", ref = "test" },
})
assert(not pcall(adapters.pi_policy.map, write, contract), "Pi writable policy must refuse unsupported native controls")
local unavailable = adapters.capabilities.new({
	provider = "pi",
	transport = supported,
	auth = supported,
	session = supported,
	permission = { available = false, reason = "native tool filters are unavailable" },
	model = supported,
	command = supported,
	tool = supported,
	context = supported,
	usage = supported,
})
assert(not pcall(adapters.pi_policy.map, read_only, unavailable), "unavailable Pi filters must fail explicitly")
local unsafe = overlay.new({
	scope = "run",
	target = "run-one",
	rules = { network_allowed = true },
	provenance = { source = "policy", ref = "test" },
})
assert(not pcall(adapters.pi_policy.map, unsafe, contract), "unsupported Pi policy translations must fail explicitly")
