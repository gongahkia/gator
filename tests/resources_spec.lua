local resources = require("gator").module("core").resources
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local root = helpers.tempdir("resources")
helpers.write(root .. "/small.txt", "abc")
assert(vim.uv.fs_symlink(root .. "/small.txt", root .. "/linked.txt") == true, "fixture must create a symbolic link")
local worktree = resources.worktree(root)
assert(
	worktree.state == "measured" and worktree.bytes == 3,
	"worktree accounting must measure regular files without following symbolic links"
)
local summary = resources.summary({
	{ workspace = { kind = "worktree", root = root } },
	{ workspace = { kind = "worktree", root = root } },
})
assert(
	summary.count == 1 and summary.state == "measured" and summary.bytes == 3,
	"worktree summary must deduplicate shared Gator worktree paths"
)
local run = resources.run({
	workspace = { kind = "project", root = root },
	resources = { started_at = 10, finished_at = 75, context_bytes = 1536, context_sends = 3 },
}, 100)
assert(
	run.wall_seconds == 65
		and run.context_bytes == 1536
		and run.context_sends == 3
		and resources.duration(run.wall_seconds) == "1m 5s"
		and resources.bytes(run.context_bytes) == "1.5 KiB",
	"resource accounting must retain wall time and Gator-delivered context bytes separately"
)
