local process = require("gator").module("adapters").process

local callbacks, timers, signals, exits = {}, {}, {}, {}
local manager = process.new({
	shutdown = false,
	timer = function()
		local timer = {
			start = function(self, delay, _, callback)
				self.delay, self.callback = delay, callback
			end,
			stop = function(self)
				self.stopped = true
			end,
			close = function(self)
				self.closed = true
			end,
		}
		table.insert(timers, timer)
		return timer
	end,
	spawn = function(_, opts, callback)
		table.insert(callbacks, callback)
		opts.stdout("abc")
		opts.stderr("def")
		return {
			pid = #callbacks,
			kill = function(_, signal)
				table.insert(signals, { pid = #callbacks, signal = signal })
				return true
			end,
		}
	end,
})

local launched = manager:launch({
	id = "captured-run",
	command = { "agent", "run" },
	timeout_ms = 25,
	max_output_bytes = 5,
	on_exit = function(status)
		table.insert(exits, status)
	end,
})
assert(
	launched.state == "running"
		and launched.output.stdout == "abc"
		and launched.output.stderr == "def"
		and not launched.output.truncated.stdout,
	"supervisor launches asynchronously and captures bounded stream output"
)
callbacks[1]({ code = 0, signal = 0 })
assert(
	manager:status("captured-run").state == "completed" and exits[1].state == "completed",
	"normal process exits must normalize to completed"
)

manager:launch({ id = "timeout-run", command = { "agent", "run" }, timeout_ms = 25 })
callbacks[2]({ code = 124, signal = 15 })
local timeout = manager:status("timeout-run")
assert(
	timeout.state == "timed_out" and timeout.failure.kind == "timeout" and timeout.failure.timeout_ms == 25,
	"timeout exits must retain a typed failure state"
)

manager:launch({ id = "cancel-run", command = { "agent", "run" }, cancel_grace_ms = 7 })
assert(manager:cancel("cancel-run").state == "cancelling", "cancellation must return without waiting for process exit")
assert(timers[1].delay == 7 and signals[#signals].signal == 15, "cancellation must send SIGTERM before escalation")
timers[1].callback()
assert(
	vim.wait(100, function()
		return signals[#signals].signal == 9
	end),
	"unresponsive cancellation must escalate to SIGKILL"
)
callbacks[3]({ code = 137, signal = 9 })
local cancelled = manager:status("cancel-run")
assert(
	cancelled.state == "cancelled" and cancelled.failure.kind == "cancelled" and timers[1].closed,
	"cancelled processes must reach a typed terminal state and release escalation timers"
)

manager:launch({ id = "shutdown-run", command = { "agent", "run" } })
local shutdown = manager:shutdown()
assert(
	#shutdown == 4 and signals[#signals - 1].signal == 15 and signals[#signals].signal == 9,
	"shutdown must terminate every managed running process"
)
callbacks[4]({ code = 137, signal = 9 })
assert(manager:status("shutdown-run").state == "cancelled", "shutdown exits must finish as cancelled")

local unavailable = process.new({
	shutdown = false,
	spawn = function()
		error("missing executable")
	end,
})
local degraded = unavailable:launch({ id = "degraded-run", command = { "missing-agent" } })
assert(
	degraded.state == "failed" and degraded.failure.kind == "spawn",
	"unavailable providers must degrade into a typed failure without blocking Neovim"
)
assert(unavailable:restart("degraded-run").failure.kind == "spawn", "degraded provider runs must remain restartable")

local real = process.new({ shutdown = false })
local started = vim.uv.hrtime()
real:launch({ id = "hung-run", command = { "sh", "-c", "sleep 1" }, timeout_ms = 25 })
assert((vim.uv.hrtime() - started) / 1000000 < 100, "hung provider launches must not block Neovim")
assert(
	vim.wait(1000, function()
		return real:status("hung-run").state == "timed_out"
	end),
	"real hung provider processes must honor supervisor timeouts"
)
assert(real:cleanup("hung-run"), "timed-out processes must be cleanup-safe")
