local process = require("gator").module("adapters").process
local core = require("gator").module("core")

local callbacks, signals = {}, {}
local manager = process.new({
	shutdown = false,
	spawn = function(_, _, callback)
		table.insert(callbacks, callback)
		return {
			pid = #callbacks,
			kill = function(_, signal)
				table.insert(signals, signal)
				return true
			end,
		}
	end,
})
manager:launch({ id = "runtime-crash", command = { "agent", "run" } })
callbacks[1]({ code = 1, signal = 0 })
assert(manager:status("runtime-crash").state == "failed", "provider crashes must become typed terminal failures")

assert(manager:restart("runtime-crash").state == "running", "crashed provider runs must be restartable")
assert(manager:cancel("runtime-crash").state == "cancelling", "restarted provider runs must remain cancellable")
callbacks[2]({ code = 137, signal = 15 })
assert(
	manager:status("runtime-crash").state == "cancelled" and signals[1] == 15,
	"cancellation must preserve typed terminal state after a restart"
)

local run = core.run.new({
	id = "run-recovery-contract",
	task_id = "task-recovery-contract",
	provider = { name = "codex", session_id = "native-recovery-contract" },
	process = { pid = 99, executable = "codex" },
	workspace = { kind = "project", root = vim.g.gator_test.root },
	state = "running",
	timing = {},
	usage = {},
})
local persisted
local reconciliation = core.run_reconciliation.reconcile({
	runs = { run },
	store = {
		put = function(_, value)
			persisted = value
			return value
		end,
	},
	alive = function()
		return false
	end,
	at = 1,
})
assert(
	reconciliation[1].status == "detached"
		and persisted.events[1].type == "run.reconciled"
		and persisted.provider.session_id == "native-recovery-contract",
	"restart recovery must retain provider-native session ownership while auditing detached processes"
)
