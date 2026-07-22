local adapters = require("gator").module("adapters")
local cost = require("gator").module("context").handoff_cost
local handoff_pack = require("gator").module("context").handoff_pack
local pack = require("gator").module("context").pack

local function contract(model)
	local ready = { available = true, modes = { "native" } }
	return adapters.capabilities.new({
		provider = "claude",
		transport = ready,
		auth = ready,
		session = ready,
		permission = ready,
		model = model,
		command = ready,
		tool = ready,
		context = ready,
		usage = ready,
	})
end

local value = handoff_pack.new({
	id = "pack-cost",
	task_id = "task-cost",
	entries = {
		pack.entry({
			id = "entry-content-cost",
			kind = "task",
			ref = "gator-task://task-cost",
			provenance = { source = "task", ref = "task-cost" },
			trust = "manual",
			token_estimate = { status = "unavailable", reason = "not counted" },
			transfer = { eligible = true },
			content = "token=fixture-secret",
		}),
		pack.entry({
			id = "entry-cached-cost",
			kind = "file",
			ref = "file://README.md",
			provenance = { source = "fixture", ref = "README.md" },
			trust = "repository",
			token_estimate = { status = "estimated", tokens = 20 },
			transfer = { eligible = true },
		}),
	},
})
local counted
local estimated = cost.estimate({
	pack = value,
	capabilities = contract({ available = true, modes = { "token_count" } }),
	count = function(text)
		counted = text
		return 10
	end,
})
assert(
	estimated.status == "estimated"
		and estimated.provider == "claude"
		and estimated.tokens == 33
		and estimated.entries[1].tokens == 11
		and estimated.entries[2].tokens == 22,
	"handoff costs must conservatively combine target-counted and cached entry estimates"
)
assert(counted:find("fixture-secret", 1, true) == nil, "target token counting must receive redacted handoff content")

local unavailable = cost.estimate({
	pack = handoff_pack.new({
		id = "pack-cost-unavailable",
		task_id = "task-cost",
		entries = {
			pack.entry({
				id = "entry-unavailable-cost",
				kind = "file",
				ref = "file://missing.txt",
				provenance = { source = "fixture", ref = "missing.txt" },
				trust = "manual",
				token_estimate = { status = "unavailable", reason = "not measured" },
				transfer = { eligible = true },
			}),
		},
	}),
	capabilities = contract({ available = true, modes = { "token_count" } }),
	count = function()
		error("must not count")
	end,
})
assert(
	unavailable.status == "unavailable" and unavailable.reason:find("entry-unavailable-cost", 1, true),
	"handoff costs must remain unavailable when an entry cannot be conservatively measured"
)
local unsupported = cost.estimate({
	pack = value,
	capabilities = contract({ available = false, reason = "provider token count unavailable" }),
})
assert(
	unsupported.status == "unavailable" and unsupported.reason:find("provider token count unavailable", 1, true),
	"handoff costs must expose unavailable target token capability"
)
assert(
	not pcall(
		cost.estimate,
		{ pack = value, capabilities = contract({ available = true, modes = { "token_count" } }), count = true }
	),
	"handoff costs must reject invalid target counters"
)
