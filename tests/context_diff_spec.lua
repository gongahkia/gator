local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local diff = require("gator").module("context").diff
local root = helpers.tempdir("context-diff")
local base = "0123456789abcdef0123456789abcdef01234567"
local patch =
	"diff --git a/file.txt b/file.txt\nindex 1111111..2222222 100644\n--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new\n"
local calls = {}
local function run(argv, cwd)
	table.insert(calls, { argv = argv, cwd = cwd })
	if argv[2] == "rev-parse" and argv[3] == "--show-toplevel" then
		return { code = 0, stdout = root .. "\n" }
	end
	if argv[2] == "rev-parse" and argv[3] == "--verify" then
		return { code = 0, stdout = base .. "\n" }
	end
	return { code = 0, stdout = patch }
end

local staged = diff.capture({ cwd = root, mode = "staged", run = run })
assert(
	staged.root == vim.uv.fs_realpath(root) and staged.mode == "staged" and staged.base == base,
	"staged diffs must retain canonical root and base commit"
)
assert(
	staged.patch == patch and staged.digest == vim.fn.sha256(patch),
	"diff capture must preserve content-addressed patch data"
)
assert(
	staged.entry.id == "diff-" .. staged.digest
		and staged.entry.provenance.ref == base
		and staged.entry.ref:find("#base=" .. base .. "#sha256=" .. staged.digest, 1, true),
	"diff entries must retain base commit provenance"
)
assert(vim.tbl_contains(calls[3].argv, "--cached"), "staged capture must use the Git index")

calls = {}
local unstaged = diff.capture({ cwd = root, mode = "unstaged", run = run })
assert(
	not vim.tbl_contains(calls[3].argv, "--cached") and unstaged.mode == "unstaged",
	"unstaged capture must omit the Git index flag"
)

calls = {}
local worktree = diff.capture({ cwd = root, mode = "worktree", base = "task-base", run = run })
assert(
	calls[3].argv[6] == base and worktree.mode == "worktree",
	"worktree capture must compare against the resolved explicit base"
)
assert(
	not pcall(diff.capture, { cwd = root, mode = "worktree", run = run }),
	"worktree capture must require base provenance"
)
assert(not pcall(diff.capture, {
	cwd = root,
	mode = "staged",
	run = function()
		return { code = 0, stdout = "" }
	end,
}), "empty Git output must fail explicitly")
