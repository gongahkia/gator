local automatic = require("gator").module("context").automatic
local config = require("gator.config")
local pack = require("gator").module("context").pack
local trust = require("gator").module("context").trust

local value = pack.new({
	id = "pack-trust",
	task_id = "task-trust",
	entries = {
		{
			id = "entry-provenance",
			kind = "file",
			ref = "provenance",
			provenance = { source = "buffer", ref = "1" },
			trust = "provenance",
			token_estimate = { status = "estimated", tokens = 1 },
			transfer = { eligible = true },
		},
		{
			id = "entry-repository",
			kind = "file",
			ref = "repository",
			provenance = { source = "repository", ref = "HEAD" },
			trust = "repository",
			token_estimate = { status = "estimated", tokens = 1 },
			transfer = { eligible = true },
		},
		{
			id = "entry-manual",
			kind = "file",
			ref = "manual",
			provenance = { source = "operator", ref = "1" },
			trust = "manual",
			token_estimate = { status = "estimated", tokens = 1 },
			transfer = { eligible = true },
		},
	},
})
local repository, decisions = trust.evaluate({ pack = value, mode = "repository" })
assert(
	decisions[1].allowed
		and decisions[2].allowed
		and not decisions[3].allowed
		and repository.entries[2].policy_decision == "allowed by repository trust",
	"repository trust must include provenance and repository context with visible decisions"
)
local selected, audit = automatic.attach({ pack = repository, policy = trust.policy("repository") })
assert(
	#selected.entries == 2
		and selected.entries[2].policy_decision == "allowed by repository trust"
		and not audit[3].allowed,
	"automatic context must preserve visible trust decisions for submitted entries"
)
local manual, manual_decisions = trust.evaluate({ pack = value, mode = "manual" })
local strict = automatic.attach({ pack = manual, policy = trust.policy("manual") })
assert(
	#strict.entries == 0
		and not manual_decisions[1].allowed
		and manual.entries[1].policy_decision:find("strict manual", 1, true),
	"manual trust must deny automatic transfer until the operator selects context"
)
assert(
	config.resolve({ context = { trust = "repository" } }).context.trust == "repository"
		and config.resolve({ context = { trust = "manual" } }).context.trust == "manual",
	"global settings must expose repository and manual trust modes"
)
