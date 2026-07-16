local core = require("gator").module("core")
local sdk = require("gator").module("extensions").workflow_sdk

local extension = sdk.define({
	name = "fixture-workflow",
	capabilities = { "task_template", "workflow", "review_action" },
	task_template = function(request)
		assert(request.input.title == "Fixture", "task templates must receive declared input")
		return core.task.new({ id = "workflow-task", objective = "Exercise workflow SDK", created_at = 1 })
	end,
	workflow = function(request)
		local value = core.task.from_record(request.task)
		request.task.sessions[1].id = "changed"
		return core.lifecycle.transition(value, "planned", 3)
	end,
	review_action = function(request)
		assert(request.action == "approve", "review actions must receive their action identifier")
		return core.lifecycle.transition(core.task.from_record(request.task), "merged", 6)
	end,
})
local created = extension.task_template({ input = { title = "Fixture" } })
local linked = core.session.link(
	created,
	core.session.new({
		task_id = "workflow-task",
		provider = "fixture",
		id = "native-session",
		owner = "provider",
	}),
	2
)
local planned = extension.workflow({ task = linked })
assert(
	planned.lifecycle == "planned" and planned.sessions[1].id == "native-session",
	"workflows must preserve provider-owned sessions"
)
local reviewing = core.lifecycle.transition(core.lifecycle.transition(planned, "running", 4), "awaiting_review", 5)
local reviewed = extension.review_action({ task = reviewing, action = "approve" })
assert(
	reviewed.lifecycle == "merged" and reviewed.sessions[1].owner == "provider",
	"review actions must use lifecycle transitions without changing session ownership"
)
local templates = sdk.define({
	name = "templates-only",
	capabilities = { "task_template" },
	task_template = function()
		return core.task.new({ id = "template-task", objective = "Fixture", created_at = 1 })
	end,
})
assert(not pcall(templates.workflow, { task = created }), "undeclared workflow capability must fail explicitly")
assert(not pcall(sdk.define, {
	name = "missing-workflow",
	capabilities = { "workflow" },
}), "declared capabilities must provide their callback")
local sessions = sdk.define({
	name = "session-template",
	capabilities = { "task_template" },
	task_template = function()
		return core.task.new({
			id = "session-task",
			objective = "Fixture",
			sessions = { { provider = "fixture", id = "native", owner = "provider" } },
			created_at = 1,
		})
	end,
})
assert(not pcall(sessions.task_template, {}), "task templates must not create provider-owned sessions")
