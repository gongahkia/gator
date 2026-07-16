local scheduler = require("gator").module("workspace").scheduler

local starts, completes = {}, {}
local queue = scheduler.new()
local first = queue:submit({
	id = "write-one",
	start = function(run, complete)
		table.insert(starts, run.id)
		completes[run.id] = complete
	end,
})
local second = queue:submit({
	id = "write-two",
	start = function(run, complete)
		table.insert(starts, run.id)
		completes[run.id] = complete
	end,
})
assert(first.state == "running" and second.state == "queued" and #starts == 1, "default cap must queue write runs")
assert(completes["write-one"](true), "first run completion must settle once")
assert(
	queue:status("write-one").state == "completed"
		and queue:status("write-two").state == "running"
		and starts[2] == "write-two",
	"completion must release the next queued write run"
)
assert(completes["write-two"](false), "failed completion must settle once")
assert(queue:status("write-two").state == "failed", "failed write runs must remain explicit")
assert(not completes["write-two"](false), "write runs must not settle twice")
assert(not pcall(queue.submit, queue, { id = "write-one", start = function() end }), "duplicate run ids must fail")

local failed = scheduler.new({ maximum = 2 })
failed:submit({
	id = "write-error",
	start = function()
		error("launcher failure")
	end,
})
assert(failed:status("write-error").state == "failed", "launcher failures must be explicit")
local parallel = 0
failed:submit({
	id = "write-parallel-one",
	start = function()
		parallel = parallel + 1
	end,
})
failed:submit({
	id = "write-parallel-two",
	start = function()
		parallel = parallel + 1
	end,
})
assert(parallel == 2, "configured caps must start the allowed number of write runs")
