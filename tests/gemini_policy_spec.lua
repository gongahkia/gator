local adapters = require("gator").module("adapters")
local overlay = require("gator").module("policy").overlay
local supported = { available = true, modes = { "native" } }
local contract = adapters.capabilities.new({
	provider = "gemini",
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
	adapters.gemini_policy.map(read_only, contract).approval_mode == "plan",
	"read-only Gator policy must map to Gemini plan mode"
)
local write = overlay.new({
	scope = "run",
	target = "run-one",
	rules = { write_allowed = true },
	provenance = { source = "policy", ref = "test" },
})
assert(
	adapters.gemini_policy.map(write, contract).approval_mode == "default",
	"write Gator policy must retain Gemini approvals"
)
local unavailable = adapters.capabilities.new({
	provider = "gemini",
	transport = supported,
	auth = supported,
	session = supported,
	permission = { available = false, reason = "native approval modes are unavailable" },
	model = supported,
	command = supported,
	tool = supported,
	context = supported,
	usage = supported,
})
assert(not pcall(adapters.gemini_policy.map, write, unavailable), "unavailable Gemini modes must fail explicitly")
local unsafe = overlay.new({
	scope = "run",
	target = "run-one",
	rules = { network_allowed = true },
	provenance = { source = "policy", ref = "test" },
})
assert(
	not pcall(adapters.gemini_policy.map, unsafe, contract),
	"unsupported Gemini policy translations must fail explicitly"
)
