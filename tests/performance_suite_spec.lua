local suite = require("gator").module("performance").suite
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local ticks, memory = {}, {}
for index = 0, 12 do
	table.insert(ticks, index * 1000000)
end
for index = 0, 12 do
	table.insert(memory, 10 + index)
end
local cases = {}
for _, name in ipairs({ "startup", "context", "stream", "worktree", "diff", "indexer" }) do
	cases[name] = function() end
end
local path = helpers.tempdir("benchmark") .. "/report.json"
local report = suite.run({
	cases = cases,
	clock = function()
		return table.remove(ticks, 1)
	end,
	memory = function()
		return table.remove(memory, 1)
	end,
	path = path,
})
assert(
	#report.metrics == 6
		and report.schema_version == 2
		and report.metrics[1].name == "startup"
		and report.metrics[6].name == "indexer"
		and vim.fn.filereadable(path) == 1,
	"benchmark suite must emit repeatable CI artifacts for every required workload"
)
assert(not pcall(suite.run, { cases = { startup = function() end } }), "benchmark suites must require every workload")
