local health = require("gator").module("indexer").health
local lifecycle = {
	status = function()
		return { state = "failed" }
	end,
	recover = function()
		return { state = "running" }
	end,
}
local fallback = health.check({ lifecycle = lifecycle })
assert(
	not fallback.available and fallback.mode == "lexical_manual" and fallback.repair.action == "restart-indexer",
	"failed indexers must expose explicit lexical/manual fallback and repair"
)
assert(health.repair({ lifecycle = lifecycle }).state == "running", "indexer health repair must recover the sidecar")
local running = health.check({ lifecycle = {
	status = function()
		return { state = "running" }
	end,
} })
assert(running.available and running.mode == "indexer", "running indexers must remain available")
