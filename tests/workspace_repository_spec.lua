local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local git = require("gator").module("core").git
local repository = require("gator").module("workspace").repository
local root = helpers.tempdir("workspace-repository")
assert(vim.fn.mkdir(root .. "/.git", "p") == 1, "repository fixture must create Git common directory")
local value = repository.detect({
	cwd = root,
	git = git.new({
		run = function(argv)
			if argv[3] == "--is-inside-work-tree" then
				return { code = 0, stdout = "true\n" }
			end
			if argv[3] == "--show-toplevel" then
				return { code = 0, stdout = root .. "\n" }
			end
			if argv[3] == "--git-common-dir" then
				return { code = 0, stdout = root .. "/.git\n" }
			end
			if argv[2] == "branch" then
				return { code = 0, stdout = "main\n" }
			end
			return { code = 0, stdout = "0123456789abcdef0123456789abcdef01234567\n" }
		end,
	}),
})
assert(
	value.root == vim.uv.fs_realpath(root)
		and value.branch == "main"
		and not value.worktree
		and #value.base_revision == 40,
	"repository detection must preserve deterministic Git identity"
)
