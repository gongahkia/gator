local git_boundary = require("gator.core.git")

local M = {}

local function fail(message)
	error("Gator workspace worktree: " .. message, 3)
end

local function invoke(git, argv, cwd, name, cancelled)
	local ok, result = pcall(git.run, git, argv, cwd, { cancelled = cancelled })
	if not ok or type(result) ~= "table" then
		fail(name .. " failed")
	end
	if result.state == "unavailable" then
		fail(name .. " is unavailable")
	end
	if result.state == "cancelled" then
		fail(name .. " was cancelled")
	end
	if result.state ~= "completed" or result.code ~= 0 then
		fail(name .. " failed")
	end
end

local function rollback(git, root, path, branch)
	local removed = pcall(invoke, git, { "git", "worktree", "remove", "--force", path }, root, "worktree rollback")
	if not removed then
		return false
	end
	return pcall(invoke, git, { "git", "branch", "--delete", "--force", branch }, root, "worktree branch rollback")
end

function M.create(opts)
	if
		type(opts) ~= "table"
		or type(opts.root) ~= "string"
		or opts.root == ""
		or type(opts.path) ~= "string"
		or opts.path == ""
		or type(opts.branch) ~= "string"
		or opts.branch == ""
		or type(opts.base) ~= "string"
		or opts.base == ""
		or (opts.run ~= nil and type(opts.run) ~= "function")
		or (opts.git ~= nil and not git_boundary.is(opts.git))
		or (opts.launch ~= nil and type(opts.launch) ~= "function")
		or (opts.cancelled ~= nil and type(opts.cancelled) ~= "function")
	then
		fail("create requires root, path, branch, base, and optional Git boundary, launch, and cancellation check")
	end
	if opts.run ~= nil and opts.git ~= nil then
		fail("create accepts either run or git")
	end
	if not opts.branch:match("^[A-Za-z0-9][A-Za-z0-9._/-]*$") then
		fail("branch is invalid")
	end
	local root = vim.uv.fs_realpath(opts.root)
	local parent = vim.uv.fs_realpath(vim.fn.fnamemodify(opts.path, ":h"))
	if not root or not parent or vim.fn.isdirectory(root) ~= 1 then
		fail("root and worktree parent must exist")
	end
	local path = parent .. "/" .. vim.fn.fnamemodify(opts.path, ":t")
	if vim.uv.fs_stat(path) then
		fail("worktree path already exists")
	end
	local git = opts.git or git_boundary.new({ run = opts.run })
	invoke(
		git,
		{ "git", "worktree", "add", "-b", opts.branch, path, opts.base },
		root,
		"worktree creation",
		opts.cancelled
	)
	if opts.launch then
		local launched, value = pcall(opts.launch, path)
		if not launched or value == false then
			if not rollback(git, root, path, opts.branch) then
				fail("worktree rollback failed")
			end
			fail("worktree launch failed")
		end
	end
	return { root = root, path = path, branch = opts.branch, base = opts.base }
end
return M
