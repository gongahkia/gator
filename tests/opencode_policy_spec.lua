local adapters = require("gator").module("adapters")
local overlay = require("gator").module("policy").overlay
local supported = { available = true, modes = { "native" } }
local contract = adapters.capabilities.new({
	provider = "opencode",
	transport = supported,
	auth = supported,
	session = supported,
	permission = { available = true, modes = { "mode" } },
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
	adapters.opencode_policy.map(read_only, contract).mode_id == "plan",
	"read-only OpenCode policy must use the native plan mode"
)
local write = overlay.new({
	scope = "run",
	target = "run-one",
	rules = { write_allowed = true },
	provenance = { source = "policy", ref = "test" },
})
assert(
	adapters.opencode_policy.map(write, contract).mode_id == "build",
	"writable OpenCode policy must retain the native build mode"
)
local unavailable = adapters.capabilities.new({
	provider = "opencode",
	transport = supported,
	auth = supported,
	session = supported,
	permission = { available = false, reason = "native modes are unavailable" },
	model = supported,
	command = supported,
	tool = supported,
	context = supported,
	usage = supported,
})
assert(not pcall(adapters.opencode_policy.map, write, unavailable), "unavailable OpenCode modes must fail explicitly")
local unsafe = overlay.new({
	scope = "run",
	target = "run-one",
	rules = { network_allowed = true },
	provenance = { source = "policy", ref = "test" },
})
assert(
	not pcall(adapters.opencode_policy.map, unsafe, contract),
	"unsupported OpenCode policy translations must fail explicitly"
)
