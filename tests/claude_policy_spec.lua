local adapters = require("gator").module("adapters")
local overlay = require("gator").module("policy").overlay
local supported = { available = true, modes = { "native" } }
local contract = adapters.capabilities.new({
	provider = "claude",
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
	adapters.claude_policy.map(read_only, contract).permission_mode == "plan",
	"read-only policy must map to Claude plan mode"
)
local write = overlay.new({
	scope = "run",
	target = "run-one",
	rules = { write_allowed = true },
	provenance = { source = "policy", ref = "test" },
})
assert(
	adapters.claude_policy.map(write, contract).permission_mode == "default",
	"write policy must map to Claude default mode"
)
local unsafe = overlay.new({
	scope = "run",
	target = "run-one",
	rules = { network_allowed = true },
	provenance = { source = "policy", ref = "test" },
})
local ok = pcall(adapters.claude_policy.map, unsafe, contract)
assert(not ok, "unsupported Claude policy translations must fail explicitly")
