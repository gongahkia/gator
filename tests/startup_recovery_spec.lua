local gator = require("gator").setup()
local coordinator = require("gator.coordinator")
local run = gator.module("core").run
local startup = gator.module("startup")

local function agent(id, state, session_id)
	return run.new({
		id = id,
		task_id = "task-startup",
		provider = session_id and { name = "codex", session_id = session_id } or { name = "codex" },
		process = { pid = 1, executable = "codex" },
		workspace = { kind = "project", root = vim.g.gator_test.root },
		state = state,
		timing = {},
		usage = {},
	})
end

local recovered = startup.recover({
	state = gator._state,
	load = function()
		return {
			agent("run-terminal", "cancelled", "native-terminal"),
			agent("run-pending", "running", "native-pending"),
		}
	end,
})
assert(
	recovered.status == "recovering"
		and recovered.runs[1].status == "terminal"
		and recovered.runs[2].status == "recovery_pending"
		and recovered.runs[2].session_id == "native-pending",
	"startup recovery must retain provider-native session references without reconnecting providers"
)
assert(
	gator._state.workspace.status == "recovering" and gator._state.workspace.detail:find("1 nonterminal", 1, true),
	"startup recovery must expose pending recovery before opening the cockpit"
)

local original_open = run.open
run.open = function()
	return {
		list = function()
			return { agent("run-coordinator", "running", "native-coordinator") }
		end,
	}
end
local value = coordinator.new()
local bootstrapped = value:bootstrap_recovery()
run.open = original_open
assert(
	bootstrapped.status == "recovering"
		and bootstrapped.runs[1].session_id == "native-coordinator"
		and value:state().workspace.status == "recovering",
	"the coordinator must bootstrap local recovery before opening the cockpit"
)

local failed = startup.recover({
	state = gator._state,
	load = function()
		error("token: private-value")
	end,
})
assert(
	failed.status == "failed" and gator._state.workspace.status == "failed" and not failed.detail:find("private%-value"),
	"startup recovery failures must stay visible and redacted"
)
local unavailable = startup.recover({
	state = gator._state,
	load = function()
		return { {} }
	end,
})
assert(
	unavailable.status == "failed" and gator._state.workspace.status == "failed",
	"startup recovery must report unavailable run records explicitly"
)
