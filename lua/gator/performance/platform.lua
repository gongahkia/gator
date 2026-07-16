local M = { api_version = 1 }

local function fail(message)
	error("Gator platform preflight: " .. message, 3)
end

local function platform(value)
	if type(value) ~= "table" or type(value.sysname) ~= "string" then
		fail("uname must provide sysname")
	end
	if value.sysname == "Darwin" then
		return "macos"
	end
	if value.sysname == "Linux" then
		local release = type(value.release) == "string" and value.release:lower() or ""
		if release:find("microsoft", 1, true) or release:find("wsl", 1, true) then
			return "wsl"
		end
		return "linux"
	end
	return "unsupported"
end

local function check(result, name, available, reason, repair)
	table.insert(result, { name = name, available = available, reason = reason, repair = repair })
end

function M.inspect(opts)
	if opts == nil then
		opts = {}
	end
	if type(opts) ~= "table" then
		fail("inspect requires options")
	end
	for key in pairs(opts) do
		if key ~= "uname" and key ~= "cwd" and key ~= "executable" and key ~= "writable" and key ~= "run" then
			fail("inspect contains unsupported field: " .. tostring(key))
		end
	end
	local uname = opts.uname or vim.uv.os_uname()
	local name = platform(uname)
	if opts.cwd ~= nil and (type(opts.cwd) ~= "string" or opts.cwd == "") then
		fail("cwd must be a non-empty string")
	end
	if opts.executable ~= nil and type(opts.executable) ~= "function" then
		fail("executable must be a function")
	end
	if opts.writable ~= nil and type(opts.writable) ~= "function" then
		fail("writable must be a function")
	end
	if opts.run ~= nil and type(opts.run) ~= "function" then
		fail("run must be a function")
	end
	local cwd = opts.cwd or vim.fn.getcwd()
	local executable = opts.executable or function(binary)
		return vim.fn.executable(binary) == 1
	end
	local writable = opts.writable or function(path)
		return vim.fn.filewritable(path) == 2
	end
	local run = opts.run
		or function(argv, path)
			local value = vim.system(argv, { cwd = path, text = true }):wait()
			return { code = value.code, stdout = value.stdout or "" }
		end
	local checks = {}
	check(checks, "platform", name ~= "unsupported", name, "Use macOS, Linux, or WSL.")
	for _, binary in ipairs({ "git", "gator-index" }) do
		check(
			checks,
			"executable." .. binary,
			executable(binary),
			binary .. " executable is unavailable",
			"Install " .. binary .. " and ensure it is on PATH."
		)
	end
	local path = vim.uv.fs_realpath(cwd)
	check(
		checks,
		"filesystem",
		path ~= nil and writable(cwd),
		"workspace is not writable",
		"Use a writable local workspace."
	)
	local git_ok, git = pcall(run, { "git", "rev-parse", "--is-inside-work-tree" }, cwd)
	local worktree = git_ok and type(git) == "table" and git.code == 0 and vim.trim(git.stdout or "") == "true"
	check(checks, "worktree", worktree, "Git worktree is unavailable", "Open a Git worktree before worktree actions.")
	return { platform = name, checks = checks }
end

return M
