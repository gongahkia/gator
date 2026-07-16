local M = {}
local function fail(message)
	error("Gator workspace worktree: " .. message, 3)
end
local function invoke(run, argv, cwd, name)
	local ok, result = pcall(run, argv, cwd)
	if not ok or type(result) ~= "table" or result.code ~= 0 then
		fail(name .. " failed")
	end
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
		or (opts.launch ~= nil and type(opts.launch) ~= "function")
	then
		fail("create requires root, path, branch, base, and optional run and launch")
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
	local run = opts.run
		or function(argv, cwd)
			local result = vim.system(argv, { cwd = cwd, text = true }):wait()
			return { code = result.code }
		end
	invoke(run, { "git", "worktree", "add", "-b", opts.branch, path, opts.base }, root, "worktree creation")
	if opts.launch then
		local launched, value = pcall(opts.launch, path)
		if not launched or value == false then
			pcall(invoke, run, { "git", "worktree", "remove", "--force", path }, root, "worktree rollback")
			fail("worktree launch failed")
		end
	end
	return { root = root, path = path, branch = opts.branch, base = opts.base }
end
return M
