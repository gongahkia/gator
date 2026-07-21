local limits = require("gator").module("core").run_limits
local run = require("gator").module("core").run

local value = run.new({
	id = "run-limits",
	task_id = "task-limits",
	provider = { name = "codex", session_id = "native-limits" },
	process = { pid = 1, executable = "codex" },
	workspace = { kind = "project", root = vim.g.gator_test.root },
	state = "running",
	timing = {},
	usage = {},
})

local decisions = {}
local timeout = limits.new({
	run = value,
	timeout_ms = 50,
	cancel = function(decision)
		table.insert(decisions, decision)
		return true
	end,
})
assert(timeout:observe({ elapsed_ms = 49 }).state == "ready", "runs below their timeout must remain active")
assert(
	timeout:observe({ elapsed_ms = 50 }).state == "cancelled"
		and decisions[1].kind == "timeout"
		and decisions[1].run_id == "run-limits",
	"timeout policy hooks must request one scoped cancellation"
)

local budget = limits.new({
	run = value,
	budget = { total_tokens = 10 },
	cancel = function()
		return true
	end,
})
assert(
	budget:observe({ usage = { total_tokens = 10 } }).kind == "budget",
	"provider usage budgets must cancel at their configured bound"
)

local unavailable = limits.new({
	run = value,
	budget = { output_tokens = 4 },
	cancel = function()
		return true
	end,
})
assert(
	unavailable:observe({ usage = { input_tokens = 1 } }).state == "unavailable",
	"missing provider usage must remain explicit"
)

local failed = limits.new({
	run = value,
	timeout_ms = 1,
	cancel = function()
		error("token: private-value")
	end,
})
assert(
	failed:observe({ elapsed_ms = 1 }).state == "failed" and not failed:status().reason:find("private%-value"),
	"cancellation-hook failures must be explicit and redacted"
)

local manual = limits.new({
	run = value,
	timeout_ms = 10,
	cancel = function()
		return true
	end,
})
assert(manual:cancel("user cancelled").kind == "cancelled", "run-limit guards must support manual cancellation")
