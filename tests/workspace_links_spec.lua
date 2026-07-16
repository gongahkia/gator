local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local links = require("gator").module("workspace").links

local path = helpers.tempdir("workspace-links") .. "/links.json"
local store = links.open(path)
local linked = store:link({
	task_id = "task-one",
	run_id = "run-one",
	worktree = { id = "worktree-one", root = "/tmp/gator-worktree" },
})
assert(
	linked.task.run_ids[1] == "run-one"
		and linked.run.task_id == "task-one"
		and linked.worktree.task_ids[1] == "task-one",
	"linking must expose task, run, and worktree references"
)
store:link({
	task_id = "task-one",
	run_id = "run-two",
	worktree = { id = "worktree-one", root = "/tmp/gator-worktree" },
})
local restarted = links.open(path)
assert(
	table.concat(restarted:task("task-one").run_ids, ",") == "run-one,run-two"
		and restarted:run("run-two").worktree_id == "worktree-one"
		and table.concat(restarted:worktree("worktree-one").run_ids, ",") == "run-one,run-two",
	"bidirectional references must survive reopening the store"
)
assert(not pcall(restarted.link, restarted, {
	task_id = "task-two",
	run_id = "run-one",
	worktree = { id = "worktree-two", root = "/tmp/gator-worktree-two" },
}), "runs must not be relinked to a different task or worktree")
