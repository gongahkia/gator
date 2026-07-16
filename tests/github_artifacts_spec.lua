local artifacts = require("gator").module("github").artifacts
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local task = require("gator").module("core").task

local root = helpers.tempdir("github-artifacts")
local value = task.new({
	id = "task-shared",
	objective = "Ship token=task-secret safely",
	sessions = { { provider = "codex", id = "native-session", owner = "provider" } },
	evidence = { { kind = "review", ref = "review-1" } },
	created_at = 1,
	updated_at = 1,
})
local artifact = artifacts.write({
	cwd = root,
	task = value,
	plan = { "review token=plan-secret", "merge" },
	handoff = "Continue with api_key=handoff-secret",
	review_evidence = { task_id = "task-shared", approvals = { { approved = true } } },
	opt_in = true,
	run = function(argv)
		if argv[2] == "rev-parse" then
			return { code = 0, stdout = root .. "\n" }
		end
		assert(argv[2] == "check-ignore", "shared artifacts must check Git ignore state")
		return { code = 1, stdout = "" }
	end,
})
local document = vim.json.decode(table.concat(vim.fn.readfile(artifact.path), "\n"))
assert(
	artifact.ref == ".gator/tasks/task-shared.json"
		and document.schema_version == 1
		and document.task.id == "task-shared"
		and document.task.sessions == nil
		and document.plan[1]:find("plan%-secret") == nil
		and document.handoff:find("handoff%-secret") == nil,
	"opted-in artifacts must serialize redacted plans and handoffs without provider session ownership"
)
assert(not pcall(artifacts.write, {
	cwd = root,
	task = value,
	run = function()
		return { code = 0, stdout = root }
	end,
}), "shared artifact writing must require explicit opt-in")
assert(not pcall(artifacts.write, {
	cwd = root,
	task = value,
	opt_in = true,
	run = function(argv)
		if argv[2] == "rev-parse" then
			return { code = 0, stdout = root }
		end
		return { code = 0, stdout = "" }
	end,
}), "Git-ignored shared artifact paths must fail explicitly")
