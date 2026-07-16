local run = require("gator").module("core").run
local recovery = require("gator").module("workspace").recovery

local function agent(id, state, session_id)
	return run.new({
		id = id,
		task_id = "task-recovery",
		provider = session_id and { name = "codex", session_id = session_id } or { name = "codex" },
		process = { pid = 1, executable = "codex" },
		workspace = { kind = "worktree", root = "/tmp/gator-recovery" },
		state = state,
		timing = {},
		usage = {},
	})
end

local reconnected = {}
local value = recovery.recover({
	runs = {
		agent("run-live", "running", "native-live"),
		agent("run-lost", "running", "native-lost"),
		agent("run-no-session", "running"),
		agent("run-complete", "completed", "native-complete"),
	},
	probe = function(reference)
		return { live = reference.run_id ~= "run-lost", resumable = true }
	end,
	reconnect = function(reference)
		reconnected[reference.run_id] = reference.session_id
		return reference.session_id == "native-live"
	end,
})
assert(
	value[1].status == "reconnected" and reconnected["run-live"] == "native-live",
	"recovery must retain native session identity"
)
assert(value[2].status == "interrupted", "missing agent processes must be classified as interrupted")
assert(
	value[3].status == "orphaned" and value[3].reason == "provider session is unavailable",
	"missing sessions must not be replaced"
)
assert(value[4].status == "terminal", "terminal runs must not be restarted")
