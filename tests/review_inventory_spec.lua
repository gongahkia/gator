local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local inventory = require("gator").module("review").inventory

local root = helpers.tempdir("review-inventory")
local value = inventory.build({
	task_id = "task-review",
	workspace = { id = "worktree-review", kind = "worktree", root = root },
	run = function(argv, cwd)
		assert(argv[1] == "git" and cwd == vim.uv.fs_realpath(root), "inventory must inspect the selected workspace")
		if argv[2] == "status" then
			return { code = 0, stdout = " M lua/gator/init.lua\0R  lua/new.lua\0lua/old.lua\0?? dist/bundle.js\0" }
		end
		assert(argv[2] == "diff" and argv[5] == "HEAD", "inventory must derive tracked-file hunks from Git diff")
		return { code = 0, stdout = "@@ -3,2 +3,4 @@\n fixture\n" }
	end,
})
assert(
	value.task_id == "task-review" and value.workspace.id == "worktree-review",
	"inventory must retain task and workspace provenance"
)
assert(
	value.files[1].path == "dist/bundle.js"
		and value.files[1].category == "untracked"
		and value.files[3].previous_path == "lua/old.lua"
		and value.files[3].category == "renamed",
	"inventory must retain sorted path status and rename ancestry"
)
assert(
	value.files[2].hunks[1].before_start == 3
		and value.files[2].hunks[1].after_count == 4
		and value.files[2].hunks[1].id:match("^hunk%-"),
	"tracked changed files must retain deterministic hunk ranges"
)
assert(#value.files[1].hunks == 0, "untracked files must retain an explicit empty hunk inventory")
assert(not pcall(inventory.build, {
	task_id = "task-review",
	workspace = { kind = "project", root = root },
	run = function()
		return { code = 1, stdout = "" }
	end,
}), "Git status failures must be explicit")
