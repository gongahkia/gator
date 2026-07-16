local health = require("gator").module("indexer").health
local lifecycle = require("gator").module("indexer").lifecycle
local process = require("gator").module("adapters").process
local callbacks, launches = {}, {}
local manager = process.new({
	spawn = function(argv, opts, callback)
		table.insert(launches, { argv = vim.deepcopy(argv), cwd = opts.cwd })
		table.insert(callbacks, callback)
		return {
			pid = #callbacks,
			kill = function()
				return true
			end,
		}
	end,
})
local sidecar = lifecycle.new({ manager = manager, cwd = vim.g.gator_test.root, executable = "gator-index" })
assert(sidecar:start().state == "running" and #callbacks == 1, "indexer startup must not wait for sidecar output")
callbacks[1]({ code = 137, signal = 9 })
assert(sidecar:status().state == "failed", "unexpected indexer exits must be observable without blocking the UI")
local fallback = health.check({ lifecycle = sidecar })
assert(
	not fallback.available and fallback.mode == "lexical_manual" and fallback.repair.action == "restart-indexer",
	"unexpected indexer exits must retain an explicit lexical/manual fallback"
)
assert(sidecar:recover().state == "running" and #callbacks == 2, "crashed indexers must restart asynchronously")
assert(
	vim.deep_equal(launches[1], launches[2]),
	"crash recovery must preserve the original sidecar executable and workspace inputs"
)
