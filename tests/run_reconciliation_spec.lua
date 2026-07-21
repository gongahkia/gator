local core = require("gator").module("core")
local reconcile = core.run_reconciliation
local run = core.run

local function agent(id, state, session_id)
	return run.new({
		id = id,
		task_id = "task-reconciliation",
		provider = session_id and { name = "codex", session_id = session_id } or { name = "codex" },
		process = { pid = 42, executable = "codex" },
		workspace = { kind = "project", root = vim.g.gator_test.root },
		state = state,
		timing = {},
		usage = {},
	})
end

local persisted, reconnected = {}, nil
local store = {
	put = function(_, value)
		persisted[value.id] = value
		return value
	end,
}
local value = reconcile.reconcile({
	runs = {
		agent("run-reconcile-attached", "running", "native-attached"),
		agent("run-reconcile-detached", "running", "native-detached"),
		agent("run-reconcile-cancelled", "cancelled", "native-cancelled"),
	},
	store = store,
	alive = function(pid)
		return pid == 42
	end,
	session_probe = function(reference)
		return reference.id == "native-attached"
	end,
	reconnect = function(reference)
		reconnected = reference
		return true
	end,
	at = 9,
})
assert(
	value[1].status == "reconnected" and reconnected.owner == "provider" and reconnected.id == "native-attached",
	"restart reconciliation must reconnect only provider-owned live sessions"
)
assert(value[2].status == "orphaned", "unavailable provider sessions must reconcile as orphaned")
assert(value[3].status == "cancelled", "cancelled runs must remain explicit during reconciliation")
assert(
	persisted["run-reconcile-attached"].events[1].type == "run.reconciled"
		and persisted["run-reconcile-attached"].events[1].at == 9,
	"reconciliation outcomes must persist a bounded run audit event"
)

local detached = reconcile.reconcile({
	runs = { agent("run-reconcile-dead", "running", "native-dead") },
	store = store,
	alive = function()
		return false
	end,
	at = 10,
})
assert(detached[1].status == "detached", "dead managed processes must reconcile as detached")

local failed = reconcile.reconcile({
	runs = { agent("run-reconcile-failed", "running", "native-failed") },
	store = store,
	alive = function()
		error("token: private-value")
	end,
	at = 11,
})
assert(
	failed[1].status == "failed" and not failed[1].reason:find("private%-value"),
	"reconciliation probe failures must be explicit and redacted"
)

local unavailable = reconcile.reconcile({
	runs = { agent("run-reconcile-unavailable", "running", "native-unavailable") },
	store = store,
	alive = function()
		return true
	end,
	at = 12,
})
assert(unavailable[1].status == "unavailable", "missing session probes must remain explicit")
