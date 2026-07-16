local pack = require("gator.context.pack")
local M = {}
local modes = { staged = true, unstaged = true, worktree = true }

local function fail(message)
	error("Gator diff context: " .. message, 3)
end

local function directory(value)
	if type(value) ~= "string" or value == "" then
		fail("cwd must be a non-empty directory")
	end
	local path = vim.uv.fs_realpath(value)
	if not path or vim.fn.isdirectory(path) ~= 1 then
		fail("cwd must be a non-empty directory")
	end
	return path
end

local function invoke(callback, argv, cwd, name)
	local ok, result = pcall(callback, argv, cwd)
	if not ok or type(result) ~= "table" or result.code ~= 0 or type(result.stdout) ~= "string" then
		fail(name .. " failed")
	end
	return result.stdout
end

local function line(value, name)
	value = vim.trim(value)
	if value == "" then
		fail(name .. " returned no value")
	end
	return value
end

local function mode(value)
	if type(value) ~= "string" or not modes[value] then
		fail("mode must be staged, unstaged, or worktree")
	end
	return value
end

local function base(value)
	if value == nil then
		return "HEAD"
	end
	if type(value) ~= "string" or value == "" then
		fail("base must be a non-empty commit reference")
	end
	return value
end

local function command(value, resolved_base)
	local argv = { "git", "diff", "--no-ext-diff", "--no-textconv", "--binary" }
	if value == "staged" then
		table.insert(argv, "--cached")
	elseif value == "worktree" then
		table.insert(argv, resolved_base)
	end
	table.insert(argv, "--")
	return argv
end

function M.capture(opts)
	if type(opts) ~= "table" then
		fail("capture requires options")
	end
	for key in pairs(opts) do
		if key ~= "cwd" and key ~= "mode" and key ~= "base" and key ~= "run" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	if opts.run ~= nil and type(opts.run) ~= "function" then
		fail("run must be a function")
	end
	local run = opts.run
		or function(argv, cwd)
			local result = vim.system(argv, { cwd = cwd, text = true }):wait()
			return { code = result.code, stdout = result.stdout or "" }
		end
	local cwd = directory(opts.cwd)
	local requested_mode = mode(opts.mode)
	if requested_mode == "worktree" and opts.base == nil then
		fail("worktree capture requires an explicit base commit")
	end
	local root = directory(
		line(invoke(run, { "git", "rev-parse", "--show-toplevel" }, cwd, "repository lookup"), "repository lookup")
	)
	local requested_base = base(opts.base)
	local resolved_base = line(
		invoke(run, { "git", "rev-parse", "--verify", requested_base .. "^{commit}" }, cwd, "base commit lookup"),
		"base commit lookup"
	)
	local patch = invoke(run, command(requested_mode, resolved_base), cwd, "diff capture")
	if patch == "" then
		fail("diff capture returned no changes")
	end
	local digest = vim.fn.sha256(patch)
	local reference = "git-diff://"
		.. root
		.. "#mode="
		.. requested_mode
		.. "#base="
		.. resolved_base
		.. "#sha256="
		.. digest
	return {
		entry = pack.entry({
			id = "diff-" .. digest,
			kind = "diff",
			ref = reference,
			provenance = { source = "git", ref = resolved_base },
			trust = "repository",
			token_estimate = { status = "unavailable", reason = "diff content has not been provider-counted" },
			transfer = { eligible = true },
		}),
		root = root,
		mode = requested_mode,
		base = resolved_base,
		digest = digest,
		patch = patch,
	}
end

return M
