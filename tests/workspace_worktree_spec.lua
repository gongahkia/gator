local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local worktree = require("gator").module("workspace").worktree
local root = helpers.tempdir("workspace-worktree")
local parent = helpers.tempdir("workspace-worktree-parent")
local calls = {}
local value = worktree.create({
	root = root,
	path = parent .. "/task-one",
	branch = "gator/task-one",
	base = "HEAD",
	run = function(argv)
		table.insert(calls, argv)
		return { code = 0 }
	end,
})
assert(
	value.branch == "gator/task-one"
		and vim.fn.fnamemodify(value.path, ":t") == "task-one"
		and calls[1][3] == "add"
		and calls[1][5] == "gator/task-one",
	"worktree creation must retain named branch and path"
)
local occupied = parent .. "/occupied"
assert(vim.fn.mkdir(occupied, "p") == 1, "must create occupied worktree path")
local before = #calls
assert(not pcall(worktree.create, {
	root = root,
	path = occupied,
	branch = "gator/occupied",
	base = "HEAD",
	run = function(argv)
		table.insert(calls, argv)
		return { code = 0 }
	end,
}), "existing worktree paths must be rejected")
assert(#calls == before, "collision checks must run before Git worktree creation")
local ok = pcall(worktree.create, {
	root = root,
	path = parent .. "/task-two",
	branch = "gator/task-two",
	base = "HEAD",
	run = function(argv)
		table.insert(calls, argv)
		return { code = 0 }
	end,
	launch = function()
		return false
	end,
})
assert(not ok and calls[#calls][3] == "remove", "worktree launch failure must roll back the created worktree")
