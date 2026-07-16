local run = require("gator").module("core").run
local session = require("gator").module("core").session
local feedback = require("gator").module("review").feedback

local agent_run = run.new({
	id = "run-feedback",
	task_id = "task-feedback",
	provider = { name = "codex", session_id = "native-feedback" },
	process = { pid = 1, executable = "codex" },
	workspace = { kind = "worktree", root = "/tmp/gator-feedback" },
	state = "completed",
	timing = {},
	usage = {},
})
local linked =
	session.new({ task_id = "task-feedback", provider = "codex", id = "native-feedback", owner = "provider" })
local routed = feedback.route({
	run = agent_run,
	session = linked,
	feedback = { { path = "lua/gator/init.lua", hunk_id = "hunk-one", decision = "accepted", annotation = "ship it" } },
	send = function(reference, payload)
		assert(
			reference.id == "native-feedback" and reference.owner == "provider",
			"feedback must retain native session ownership"
		)
		assert(
			payload.context.workspace.kind == "worktree" and payload.context.files[1] == "lua/gator/init.lua",
			"feedback must include workspace context"
		)
		return true
	end,
})
assert(routed.feedback[1].decision == "accepted", "selected hunk decisions must be routed structurally")
local other = session.new({ task_id = "task-feedback", provider = "codex", id = "native-other", owner = "provider" })
assert(not pcall(feedback.route, {
	run = agent_run,
	session = other,
	feedback = { { path = "file", hunk_id = "hunk-one", decision = "accepted" } },
	send = function()
		return true
	end,
}), "feedback must not route to a replacement session")
