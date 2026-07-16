local startup = require("gator").module("startup")
local ticks = { 0, 2500000 }
local report = startup.measure({
	clock = function()
		return table.remove(ticks, 1)
	end,
	load = function()
		require("gator").setup()
	end,
})
assert(
	report.duration_ms == 2.5
		and report.process_launches == 0
		and vim.deep_equal(report.deferred_modules, { "gator.adapters", "gator.indexer", "gator.github" }),
	"startup measurement must report zero eager launches and deferred external modules"
)
assert(not pcall(startup.measure, {
	load = function()
		vim.system({ "agent", "run" })
	end,
}), "startup measurement must reject eager process launches")
