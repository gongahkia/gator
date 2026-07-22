local scheduler = require("gator").module("workspace").scheduler

local starts, completes = {}, {}
local queue = scheduler.new()
local first = queue:submit({
	id = "write-one",
	worktree = "workspace-one",
	provider = "fixture",
	start = function(run, complete)
		table.insert(starts, run.id)
		completes[run.id] = complete
	end,
})
local second = queue:submit({
	id = "write-two",
	worktree = "workspace-one",
	provider = "fixture",
	start = function(run, complete)
		table.insert(starts, run.id)
		completes[run.id] = complete
	end,
})
assert(
	first.state == "running" and second.state == "queued" and #starts == 1,
	"one active write provider must reserve its worktree"
)
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
assert(not pcall(queue.submit, queue, {
	id = "write-one",
	worktree = "workspace-one",
	provider = "fixture",
	start = function() end,
}), "duplicate run ids must fail")

local failed = scheduler.new({ maximum = 2 })
failed:submit({
	id = "write-error",
	worktree = "workspace-error",
	provider = "fixture",
	start = function()
		error("launcher failure")
	end,
})
assert(failed:status("write-error").state == "failed", "launcher failures must be explicit")
local parallel = 0
failed:submit({
	id = "write-parallel-one",
	worktree = "workspace-parallel-one",
	provider = "fixture",
	start = function()
		parallel = parallel + 1
	end,
})
failed:submit({
	id = "write-parallel-two",
	worktree = "workspace-parallel-two",
	provider = "fixture",
	start = function()
		parallel = parallel + 1
	end,
})
assert(parallel == 2, "configured caps must start the allowed number of write runs")

local isolated, callbacks = scheduler.new({ maximum = 2 }), {}
isolated:submit({
	id = "write-isolated-one",
	worktree = "workspace-isolated",
	provider = "provider-one",
	start = function(run, complete)
		callbacks[run.id] = complete
	end,
})
isolated:submit({
	id = "write-isolated-two",
	worktree = "workspace-isolated",
	provider = "provider-two",
	start = function(run, complete)
		callbacks[run.id] = complete
	end,
})
isolated:submit({
	id = "write-independent",
	worktree = "workspace-independent",
	provider = "provider-three",
	start = function(run, complete)
		callbacks[run.id] = complete
	end,
})
assert(
	isolated:status("write-isolated-one").state == "running"
		and isolated:status("write-isolated-two").state == "queued"
		and isolated:status("write-independent").state == "running",
	"different worktrees may run concurrently while one worktree remains exclusive"
)
assert(callbacks["write-isolated-one"](true), "first isolated write run must complete")
assert(
	isolated:status("write-isolated-two").state == "running",
	"releasing a worktree must start its next queued provider"
)

local cancellable = scheduler.new()
cancellable:submit({
	id = "write-cancelled",
	worktree = "workspace-cancelled",
	provider = "fixture",
	start = function() end,
	cancel = function()
		return true
	end,
})
assert(
	cancellable:cancel("write-cancelled").state == "cancelled",
	"running write runs must support explicit cancellation"
)
cancellable:submit({
	id = "write-uncancellable",
	worktree = "workspace-uncancellable",
	provider = "fixture",
	start = function() end,
})
assert(cancellable:cancel("write-uncancellable").state == "unavailable", "missing cancellation must remain explicit")
