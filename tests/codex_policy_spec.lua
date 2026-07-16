local adapters = require("gator").module("adapters")
local overlay = require("gator").module("policy").overlay
local supported = { available = true, modes = { "native" } }
local contract = adapters.capabilities.new({
	provider = "codex",
	transport = supported,
	auth = supported,
	session = supported,
	permission = { available = true, modes = { "sandbox", "approval" } },
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
	adapters.codex_policy.map(read_only, contract).sandbox_mode == "read-only",
	"read-only Gator policy must map to Codex read-only"
)
local write = overlay.new({
	scope = "run",
	target = "run-one",
	rules = { write_allowed = true },
	provenance = { source = "policy", ref = "test" },
})
assert(
	adapters.codex_policy.map(write, contract).sandbox_mode == "workspace-write",
	"write Gator policy must map to workspace-write"
)
local unsafe = overlay.new({
	scope = "run",
	target = "run-one",
	rules = { network_allowed = true },
	provenance = { source = "policy", ref = "test" },
})
local ok = pcall(adapters.codex_policy.map, unsafe, contract)
assert(not ok, "unsafe or unsupported policy rules must refuse translation")
