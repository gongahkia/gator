local process = require("gator").module("adapters").process
local lifecycle = require("gator").module("indexer").lifecycle
local callbacks = {}
local manager = process.new({
	spawn = function(_, _, callback)
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
assert(sidecar:start().state == "running" and #callbacks == 1, "indexer start must launch asynchronously")
assert(sidecar:start().pid == 1 and #callbacks == 1, "running indexer must not relaunch")
assert(sidecar:stop().state == "cancelling", "indexer stop must cancel without blocking")
callbacks[1]({ code = 0, signal = 15 })
assert(sidecar:recover().state == "running" and #callbacks == 2, "terminal indexer state must recover by restart")
