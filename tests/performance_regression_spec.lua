local suite = require("gator").module("performance").suite
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local cases = {}
for _, name in ipairs({ "startup", "context", "stream", "worktree", "diff", "indexer" }) do
	cases[name] = function() end
end

local function clocks(first_duration, sustained)
	local ticks, current = {}, 0
	for pair = 1, 18 do
		local duration = sustained or (pair == 1 and first_duration or 5)
		table.insert(ticks, current * 1000000)
		table.insert(ticks, (current + duration) * 1000000)
		current = current + 30
	end
	return function()
		return table.remove(ticks, 1)
	end
end

local budgets = {}
for _, name in ipairs({ "startup", "context", "stream", "worktree", "diff", "indexer" }) do
	budgets[name] = { duration_ms = 10, memory_kb_delta = 10, variance_percent = 0, sustained_samples = 3 }
end
local stable = suite.run({
	cases = cases,
	samples = 3,
	clock = clocks(20, nil),
	memory = function()
		return 10
	end,
	budgets = budgets,
})
assert(#stable.regressions == 0, "a one-sample breach must not fail the sustained regression gate")
local path = helpers.tempdir("performance-regression") .. "/report.json"
local failed = pcall(suite.run, {
	cases = cases,
	samples = 3,
	clock = clocks(20, 20),
	memory = function()
		return 10
	end,
	budgets = budgets,
	path = path,
})
assert(
	not failed and vim.fn.filereadable(path) == 1,
	"sustained actionable regressions must fail after writing their artifact"
)
