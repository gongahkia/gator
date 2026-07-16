local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local merge = require("gator").module("review").merge

local target = helpers.tempdir("review-merge-target")
local source = helpers.tempdir("review-merge-source")
local target_root = assert(vim.uv.fs_realpath(target))
local source_root = assert(vim.uv.fs_realpath(source))
local merges, aborted, rebased = 0, false, false
local function retry_run(argv, cwd)
	if argv[2] == "status" then
		return { code = 0, stdout = "" }
	end
	if argv[2] == "rev-parse" then
		return { code = 0, stdout = "source-sha\n" }
	end
	if argv[2] == "branch" then
		return { code = 0, stdout = "main\n" }
	end
	if argv[2] == "merge" and argv[3] == "--abort" then
		aborted = true
		return { code = 0, stdout = "" }
	end
	if argv[2] == "rebase" then
		rebased = cwd == source_root and argv[3] == "main"
		return { code = 0, stdout = "" }
	end
	if argv[2] == "merge" then
		merges = merges + 1
		return merges == 1 and { code = 1, stderr = "CONFLICT (content): merge conflict\n" }
			or { code = 0, stdout = "" }
	end
	error("unexpected Git command")
end
local value = merge.apply({
	target_root = target,
	source_root = source,
	source_branch = "gator/task",
	mode = "merge",
	retry_rebase = true,
	confirm = true,
	run = retry_run,
})
assert(
	value.status == "merged" and value.rebase_retried and aborted and rebased,
	"merge conflicts must support explicit rebase retry"
)
local conflict = merge.apply({
	target_root = target,
	source_root = source,
	source_branch = "gator/task",
	mode = "merge",
	retry_rebase = false,
	confirm = true,
	run = function(argv)
		if argv[2] == "status" then
			return { code = 0, stdout = "" }
		end
		if argv[2] == "rev-parse" then
			return { code = 0, stdout = "source-sha\n" }
		end
		if argv[2] == "branch" then
			return { code = 0, stdout = "main\n" }
		end
		return { code = 1, stderr = "CONFLICT (content): merge conflict\n" }
	end,
})
assert(conflict.status == "conflict" and conflict.phase == "merge", "unresolved merge conflicts must be reported")
assert(not pcall(merge.apply, {
	target_root = target,
	source_root = source,
	source_branch = "gator/task",
	mode = "merge",
	retry_rebase = false,
	confirm = false,
}), "merges must never run without confirmation")
