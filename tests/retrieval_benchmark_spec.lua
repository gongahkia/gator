local benchmark = require("gator").module("performance").retrieval
local ticks = { 0, 1000000, 1000000, 3000000, 3000000, 6000000 }
local memory = { 10, 12, 12, 17, 17, 20 }
local calls = {}
local value = benchmark.run({
	queries = { "first", "second" },
	clock = function()
		return table.remove(ticks, 1)
	end,
	memory = function()
		return table.remove(memory, 1)
	end,
	cancel = function(kind, query)
		return kind == "vector" and query == "second"
	end,
	lexical = function(query, control)
		assert(not control.cancelled(), "active lexical queries must not be marked cancelled")
		table.insert(calls, "lexical:" .. query)
	end,
	vector = function(query, control)
		assert(not control.cancelled(), "active vector queries must not be marked cancelled")
		table.insert(calls, "vector:" .. query)
	end,
})
assert(
	value.lexical.samples == 2
		and value.lexical.total_ms == 3
		and value.lexical.memory_kb_delta == 5
		and value.vector.samples == 1
		and value.vector.cancelled == 1
		and not vim.tbl_contains(calls, "vector:second"),
	"retrieval benchmarks must report latency, memory, and cancellation without retaining query data"
)
assert(
	not pcall(benchmark.run, { queries = { "one" }, lexical = function() end }),
	"benchmark modes must fail explicitly"
)
