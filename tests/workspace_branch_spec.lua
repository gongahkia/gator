local branch = require("gator").module("workspace").branch

local derived = branch.resolve({ task_id = "task-105" })
assert(
	derived.name == "gator/task-105"
		and derived.branch == "gator/task-105"
		and derived.worktree_name == "gator/task-105"
		and derived.source == "derived"
		and derived.task_id == "task-105",
	"derived names must retain Gator task identity"
)
local configured = branch.resolve({ task_id = "task_105", prefix = "gator/workspaces" })
assert(
	configured.name == "gator/workspaces/task_105" and configured.worktree_name == "gator/workspaces/task_105",
	"prefix must configure deterministic branch and worktree names"
)
assert(
	branch.resolve({ task_id = "task-105" }).worktree_name ~= branch.resolve({ task_id = "task_105" }).worktree_name,
	"distinct task identifiers must not collide after worktree naming"
)
local overridden = branch.resolve({ task_id = "task-105", override = "review/task-105" })
assert(
	overridden.name == "review/task-105"
		and overridden.worktree_name == "review/task-105"
		and overridden.source == "override",
	"valid overrides must preserve an explicit worktree name"
)
assert(not pcall(branch.resolve, { task_id = "Task-105" }), "task identity must be validated")
assert(not pcall(branch.resolve, { task_id = "task-105", prefix = "gator//workspaces" }), "invalid prefixes must fail")
assert(not pcall(branch.resolve, { task_id = "task-105", override = "review/.." }), "invalid overrides must fail")
assert(
	not pcall(branch.resolve, { task_id = "task-105", override = "review\\task" }),
	"Git-invalid characters must fail"
)
