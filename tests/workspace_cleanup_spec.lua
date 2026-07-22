local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local cleanup = require("gator").module("workspace").cleanup

local root = helpers.tempdir("workspace-cleanup")
local clean = root .. "/clean"
local dirty = root .. "/dirty"
local active = root .. "/active"
local locked = root .. "/locked"
assert(
	vim.fn.mkdir(clean, "p") == 1
		and vim.fn.mkdir(dirty, "p") == 1
		and vim.fn.mkdir(active, "p") == 1
		and vim.fn.mkdir(locked, "p") == 1,
	"must create worktree fixtures"
)
local resolved_root = assert(vim.uv.fs_realpath(root))
local resolved_clean = assert(vim.uv.fs_realpath(clean))
local resolved_dirty = assert(vim.uv.fs_realpath(dirty))
local resolved_active = assert(vim.uv.fs_realpath(active))
local resolved_locked = assert(vim.uv.fs_realpath(locked))
local removed = {}
local function run(argv, cwd)
	if argv[2] == "worktree" and argv[3] == "list" then
		return {
			code = 0,
			stdout = "worktree "
				.. resolved_root
				.. "\nbranch refs/heads/main\n\nworktree "
				.. resolved_clean
				.. "\nbranch refs/heads/gator/clean\n\nworktree "
				.. resolved_dirty
				.. "\nbranch refs/heads/gator/dirty\n\nworktree "
				.. resolved_active
				.. "\nbranch refs/heads/gator/active\n\nworktree "
				.. resolved_locked
				.. "\nbranch refs/heads/gator/locked\nlocked protected fixture\n",
		}
	end
	if argv[2] == "status" then
		return { code = 0, stdout = cwd == resolved_dirty and " M changed.txt\0" or "" }
	end
	if argv[2] == "worktree" and argv[3] == "remove" then
		table.insert(removed, argv[4])
		return { code = 0 }
	end
	error("unexpected Git command")
end
local manager = cleanup.new({
	root = root,
	run = run,
	active = function(path)
		return path == resolved_active
	end,
})
local plan = manager:plan()
assert(#plan == 1 and plan[1].path == resolved_clean, "only clean inactive unlocked worktrees must be recoverable")
assert(not pcall(manager.prune, manager, plan, false), "destructive cleanup must require confirmation")
assert(
	#manager:prune(plan, true) == 1 and removed[1] == resolved_clean,
	"confirmed cleanup must remove only planned worktrees"
)
