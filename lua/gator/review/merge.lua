local M = {}
local modes = { merge = true, rebase = true }

local function fail(message)
	error("Gator review merge: " .. message, 3)
end

local function directory(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be an existing directory")
	end
	local path = vim.uv.fs_realpath(value)
	if not path or vim.fn.isdirectory(path) ~= 1 then
		fail(name .. " must be an existing directory")
	end
	return path
end

local function invoke(run, argv, cwd, name)
	local ok, result = pcall(run, argv, cwd)
	if not ok or type(result) ~= "table" or type(result.code) ~= "number" or result.code % 1 ~= 0 then
		fail(name .. " failed to launch")
	end
	return { code = result.code, stdout = result.stdout, stderr = result.stderr }
end

local function output(result)
	return (result.stdout or "") .. (result.stderr or "")
end

local function conflict(result)
	return output(result):find("CONFLICT", 1, true) ~= nil
end

local function require_success(run, argv, cwd, name)
	local result = invoke(run, argv, cwd, name)
	if result.code ~= 0 then
		fail(name .. " failed: " .. output(result))
	end
	if type(result.stdout) ~= "string" then
		fail(name .. " returned no output")
	end
	return result.stdout
end

local function clean(run, cwd, name)
	if require_success(run, { "git", "status", "--porcelain=v1", "-z", "--untracked-files=all" }, cwd, name) ~= "" then
		fail(name .. " must be clean")
	end
end

local function branch(run, cwd)
	local value = vim.trim(require_success(run, { "git", "branch", "--show-current" }, cwd, "Git target branch lookup"))
	if value == "" then
		fail("target worktree must be on a branch")
	end
	return value
end

local function report(phase, result)
	return { status = conflict(result) and "conflict" or "failed", phase = phase, report = output(result) }
end

function M.apply(opts)
	if type(opts) ~= "table" then
		fail("apply requires options")
	end
	for key in pairs(opts) do
		if
			key ~= "target_root"
			and key ~= "source_root"
			and key ~= "source_branch"
			and key ~= "mode"
			and key ~= "retry_rebase"
			and key ~= "confirm"
			and key ~= "run"
		then
			fail("apply contains unsupported field: " .. tostring(key))
		end
	end
	if opts.confirm ~= true then
		fail("merge requires explicit confirmation")
	end
	if
		type(opts.source_branch) ~= "string"
		or opts.source_branch == ""
		or opts.source_branch:sub(1, 1) == "-"
		or opts.source_branch:find("[%c ]")
	then
		fail("source_branch must be a non-empty Git ref")
	end
	if type(opts.mode) ~= "string" or not modes[opts.mode] then
		fail("mode must be merge or rebase")
	end
	if type(opts.retry_rebase) ~= "boolean" then
		fail("retry_rebase must be boolean")
	end
	if opts.run ~= nil and type(opts.run) ~= "function" then
		fail("run must be a function")
	end
	local target = directory(opts.target_root, "target_root")
	local source = directory(opts.source_root, "source_root")
	if target == source then
		fail("source and target worktrees must differ")
	end
	local run = opts.run
		or function(argv, cwd)
			local result = vim.system(argv, { cwd = cwd, text = true }):wait()
			return { code = result.code, stdout = result.stdout or "", stderr = result.stderr or "" }
		end
	clean(run, target, "target worktree")
	clean(run, source, "source worktree")
	require_success(
		run,
		{ "git", "rev-parse", "--verify", opts.source_branch .. "^{commit}" },
		target,
		"Git source revision lookup"
	)
	local target_branch = branch(run, target)
	local retried = false
	local function rebase_source()
		local value = invoke(run, { "git", "rebase", target_branch }, source, "Git source rebase")
		if value.code ~= 0 then
			return nil, report("rebase", value)
		end
		return true
	end
	local function merge_source(ff_only)
		local argv = ff_only and { "git", "merge", "--ff-only", opts.source_branch }
			or { "git", "merge", "--no-ff", "--no-edit", opts.source_branch }
		return invoke(run, argv, target, "Git merge")
	end
	if opts.mode == "rebase" then
		local rebased, failure = rebase_source()
		if not rebased then
			return failure
		end
		local merged = merge_source(true)
		if merged.code ~= 0 then
			return report("merge", merged)
		end
		return {
			status = "merged",
			mode = "rebase",
			target_branch = target_branch,
			source_branch = opts.source_branch,
			rebase_retried = false,
		}
	end
	local merged = merge_source(false)
	if merged.code == 0 then
		return {
			status = "merged",
			mode = "merge",
			target_branch = target_branch,
			source_branch = opts.source_branch,
			rebase_retried = false,
		}
	end
	if not conflict(merged) or not opts.retry_rebase then
		return report("merge", merged)
	end
	require_success(run, { "git", "merge", "--abort" }, target, "Git merge abort")
	local rebased, failure = rebase_source()
	if not rebased then
		return failure
	end
	retried = true
	merged = merge_source(true)
	if merged.code ~= 0 then
		return report("merge", merged)
	end
	return {
		status = "merged",
		mode = "merge",
		target_branch = target_branch,
		source_branch = opts.source_branch,
		rebase_retried = retried,
	}
end

return M
