local git_boundary = require("gator.core.git")

local M = {}

local function fail(message)
	error("Gator workspace repository: " .. message, 3)
end

local function result(run, argv, cwd, name)
	local ok, value = pcall(run, argv, cwd)
	if not ok or type(value) ~= "table" or value.code ~= 0 or type(value.stdout) ~= "string" then
		fail(name .. " failed")
	end
	local output = vim.trim(value.stdout)
	if output == "" then
		fail(name .. " returned no value")
	end
	return output
end

function M.detect(opts)
	if
		type(opts) ~= "table"
		or type(opts.cwd) ~= "string"
		or opts.cwd == ""
		or (opts.run ~= nil and type(opts.run) ~= "function")
		or (opts.git ~= nil and not git_boundary.is(opts.git))
	then
		fail("detect requires cwd and optional Git boundary")
	end
	if opts.run ~= nil and opts.git ~= nil then
		fail("detect accepts either run or git")
	end
	local cwd = vim.uv.fs_realpath(opts.cwd)
	if not cwd or vim.fn.isdirectory(cwd) ~= 1 then
		fail("cwd must be an existing directory")
	end
	local git = opts.git or git_boundary.new({ run = opts.run })
	local run = function(argv, path)
		return git:run(argv, path)
	end
	if result(run, { "git", "rev-parse", "--is-inside-work-tree" }, cwd, "Git worktree check") ~= "true" then
		fail("cwd is not a Git worktree")
	end
	local root = vim.uv.fs_realpath(result(run, { "git", "rev-parse", "--show-toplevel" }, cwd, "Git root lookup"))
	local common =
		vim.uv.fs_realpath(result(run, { "git", "rev-parse", "--git-common-dir" }, cwd, "Git common directory lookup"))
	if not root or not common then
		fail("Git paths are unavailable")
	end
	return {
		root = root,
		common_dir = common,
		branch = result(run, { "git", "branch", "--show-current" }, cwd, "Git branch lookup"),
		base_revision = result(run, { "git", "rev-parse", "--verify", "HEAD" }, cwd, "Git base revision lookup"),
		worktree = common ~= root .. "/.git",
	}
end

return M
