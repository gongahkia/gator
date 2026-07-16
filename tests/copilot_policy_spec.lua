local adapters = require("gator").module("adapters")
local overlay = require("gator").module("policy").overlay
local supported = { available = true, modes = { "native" } }
local contract = adapters.capabilities.new({
	provider = "copilot",
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
	vim.deep_equal(adapters.copilot_policy.map(read_only, contract).available_tools, { "view", "glob", "grep" }),
	"read-only Copilot policy must use a native read-only tool allowlist"
)
local write = overlay.new({
	scope = "run",
	target = "run-one",
	rules = { write_allowed = true },
	provenance = { source = "policy", ref = "test" },
})
assert(vim.deep_equal(adapters.copilot_policy.map(write, contract), {}), "writable policy must retain native approvals")
local unavailable = adapters.capabilities.new({
	provider = "copilot",
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
assert(not pcall(adapters.copilot_policy.map, write, unavailable), "unavailable Copilot filters must fail explicitly")
local unsafe = overlay.new({
	scope = "run",
	target = "run-one",
	rules = { network_allowed = true },
	provenance = { source = "policy", ref = "test" },
})
assert(
	not pcall(adapters.copilot_policy.map, unsafe, contract),
	"unsupported Copilot policy translations must fail explicitly"
)
