local gator = require("gator").setup()
local ui = require("gator.ui")
local task = ui.create_task(gator._state, {
	id = "task-inline",
	objective = "Review capture token=fixture-secret",
	now = 7,
	captures = {
		{
			id = "capture-inline",
			kind = "selection",
			ref = "buffer://1#L1-L2",
			provenance = { source = "editor", ref = "1" },
			trust = "manual",
			token_estimate = { status = "estimated", tokens = 1 },
			transfer = { eligible = true },
			content = "not retained in task evidence",
		},
	},
})
assert(
	task.id == "task-inline"
		and task.lifecycle == "draft"
		and task.objective == "Review capture token=[REDACTED]"
		and task.evidence[1].kind == "selection"
		and task.evidence[1].ref == "buffer://1#L1-L2"
		and task.evidence[1].content == nil,
	"inline task creation must redact objectives and retain only capture evidence references"
)
assert(
	gator._state.tasks[1].id == "task-inline" and gator._state:version() == 1,
	"inline task creation must update reactive state exactly once"
)
assert(
	not pcall(ui.create_task, gator._state, { id = "task-inline", objective = "duplicate", now = 7 }),
	"inline task creation must reject duplicate identifiers"
)
assert(not pcall(ui.create_task, gator._state, {
	id = "task-invalid",
	objective = "invalid",
	captures = { { id = "capture-invalid" } },
}), "inline task creation must reject malformed capture evidence")

local input = vim.ui.input
local prompted
vim.ui.input = function(_, callback)
	callback("Create from inline prompt")
end
ui.prompt_task(gator._state, {
	id = "task-prompt",
	now = 8,
	on_created = function(value)
		prompted = value
	end,
})
vim.ui.input = input
assert(
	prompted and prompted.id == "task-prompt" and prompted.objective == "Create from inline prompt",
	"inline prompt task creation must remain non-blocking and create a draft task"
)
