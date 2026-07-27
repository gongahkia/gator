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

function M.remove(opts)
	if
		type(opts) ~= "table"
		or type(opts.root) ~= "string"
		or opts.root == ""
		or type(opts.path) ~= "string"
		or opts.path == ""
		or type(opts.branch) ~= "string"
		or opts.branch == ""
		or (opts.run ~= nil and type(opts.run) ~= "function")
		or (opts.git ~= nil and not git_boundary.is(opts.git))
	then
		fail("remove requires root, path, branch, and optional Git boundary")
	end
	if opts.run ~= nil and opts.git ~= nil then
		fail("remove accepts either run or git")
	end
	local root = vim.uv.fs_realpath(opts.root)
	local path = vim.uv.fs_realpath(opts.path)
	if not root or not path or root == path then
		fail("remove requires a linked worktree")
	end
	local git = opts.git or git_boundary.new({ run = opts.run })
	invoke(git, { "git", "worktree", "remove", "--force", path }, root, "worktree removal")
	invoke(git, { "git", "branch", "--delete", "--force", opts.branch }, root, "worktree branch removal")
	return true
end

local function lock_request(opts, action)
	if
		type(opts) ~= "table"
		or type(opts.root) ~= "string"
		or opts.root == ""
		or type(opts.path) ~= "string"
		or opts.path == ""
		or (opts.reason ~= nil and (type(opts.reason) ~= "string" or opts.reason == ""))
		or (opts.run ~= nil and type(opts.run) ~= "function")
		or (opts.git ~= nil and not git_boundary.is(opts.git))
	then
		fail(action .. " requires root, path, optional reason, and optional Git boundary")
	end
	if opts.run ~= nil and opts.git ~= nil then
		fail(action .. " accepts either run or git")
	end
	local root = vim.uv.fs_realpath(opts.root)
	local path = vim.uv.fs_realpath(opts.path)
	if not root or not path or root == path then
		fail(action .. " requires a linked worktree")
	end
	return root, path, opts.git or git_boundary.new({ run = opts.run })
end

function M.lock(opts)
	local root, path, git = lock_request(opts, "lock")
	local argv = { "git", "worktree", "lock" }
	if opts.reason then
		table.insert(argv, "--reason")
		table.insert(argv, opts.reason)
	end
	table.insert(argv, path)
	invoke(git, argv, root, "worktree lock")
	return true
end

function M.unlock(opts)
	local root, path, git = lock_request(opts, "unlock")
	invoke(git, { "git", "worktree", "unlock", path }, root, "worktree unlock")
	return true
end

return M
