local queue = require("gator").module("core").launch_queue
local runtime = require("gator").module("core").runtime

local owner = runtime.new({
	clock = function()
		return 1
	end,
})
local launches, completions, cancellations = {}, {}, {}
local value = queue.new({ runtime = owner, id = "provider-launch", limit = 2 })
assert(
	owner:status("provider-launch").state == "registered"
		and not pcall(value.enqueue, value, { id = "launch-one", provider = "codex" }),
	"provider launch queues must remain unavailable until runtime ownership starts"
)
value:start()
for _, id in ipairs({ "launch-one", "launch-two", "launch-three" }) do
	value:enqueue({
		id = id,
		provider = "codex",
		launch = function(done)
			launches[#launches + 1] = id
			completions[id] = done
			return { id = id }
		end,
		cancel = function(_, reason)
			cancellations[id] = reason
			return true
		end,
	})
end
assert(
	vim.deep_equal(launches, { "launch-one", "launch-two" }) and value:status("launch-three").state == "queued",
	"provider launch queues must enforce the configured concurrent-process bound"
)
completions["launch-one"]("completed")
assert(
	vim.deep_equal(launches, { "launch-one", "launch-two", "launch-three" })
		and value:status("launch-one").state == "completed"
		and value:status("launch-three").state == "running",
	"provider launch queues must start pending work when a bounded slot becomes available"
)
assert(
	value:cancel("launch-three", "user-cancel").state == "cancelling",
	"running provider launches must be cancellable"
)
completions["launch-three"]("cancelled")
assert(
	cancellations["launch-three"] == "user-cancel" and value:status("launch-three").state == "cancelled",
	"provider launch queues must retain cancellation outcomes without provider credentials"
)
value:enqueue({
	id = "launch-failed",
	provider = "codex",
	launch = function()
		error("token: private-value")
	end,
	cancel = function()
		return true
	end,
})
assert(value:status("launch-failed").state == "failed", "provider launch failures must become terminal queue outcomes")
assert(
	value:stop("cancelled").state == "cancelled" and owner:status("provider-launch").state == "cancelled",
	"provider launch queues must stop through their runtime owner"
)
