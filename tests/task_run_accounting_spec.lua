local accounting = require("gator").module("core").task_run_accounting
local run = require("gator").module("core").run

local function agent(id, task_id, state)
	return run.new({
		id = id,
		task_id = task_id,
		provider = { name = "codex", session_id = "native-" .. id },
		process = { pid = 1, executable = "codex" },
		workspace = { kind = "project", root = vim.g.gator_test.root },
		state = state,
		timing = {},
		usage = {},
	})
end

local value = accounting.new({ maximum = 1 })
assert(
	value:observe(agent("run-account-running", "task-account", "running")).state == "saturated",
	"running runs must consume their task concurrency slot"
)
local queued = value:observe(agent("run-account-queued", "task-account", "queued"))
assert(
	queued.running == 1 and queued.queued == 1 and not value:can_start("task-account").allowed,
	"queued runs must remain accounted without exceeding task concurrency"
)
assert(
	value:observe(agent("run-other-task", "task-other", "running")).task_id == "task-other",
	"different tasks must maintain independent concurrency state"
)
local cancelled = value:cancel("run-account-running")
assert(
	cancelled.changed and cancelled.task.state == "available" and cancelled.task.cancelled == 1,
	"cancelled runs must release their task concurrency slot"
)
assert(
	value:observe(agent("run-account-failed", "task-account", "failed")).failed == 1,
	"failed runs must remain explicit in task concurrency state"
)
assert(
	not value:can_start("missing-task").allowed and value:status("missing-task").state == "unavailable",
	"unknown task run state must remain explicit"
)
assert(not value:cancel("missing-run").changed, "unavailable run cancellation must not fabricate accounting state")
