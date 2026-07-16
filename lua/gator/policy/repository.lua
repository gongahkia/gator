local overlay = require("gator.policy.overlay")
local M = {}
local artifact = ".gator/repository-policy.json"

local function fail(message)
	error("Gator repository policy: " .. message, 3)
end

local function invoke(run, argv, cwd, name)
	local ok, result = pcall(run, argv, cwd)
	if not ok or type(result) ~= "table" or type(result.code) ~= "number" or result.code % 1 ~= 0 then
		fail(name .. " failed")
	end
	return { code = result.code, stdout = result.stdout }
end

function M.load(opts)
	if type(opts) ~= "table" or type(opts.cwd) ~= "string" or opts.cwd == "" then
		fail("load requires cwd")
	end
	for key in pairs(opts) do
		if key ~= "cwd" and key ~= "run" then
			fail("load contains unsupported field: " .. tostring(key))
		end
	end
	if opts.run ~= nil and type(opts.run) ~= "function" then
		fail("run must be a function")
	end
	local cwd = vim.uv.fs_realpath(opts.cwd)
	if not cwd or vim.fn.isdirectory(cwd) ~= 1 then
		fail("cwd must be an existing directory")
	end
	local run = opts.run
		or function(argv, path)
			local result = vim.system(argv, { cwd = path, text = true }):wait()
			return { code = result.code, stdout = result.stdout or "" }
		end
	local root_result = invoke(run, { "git", "rev-parse", "--show-toplevel" }, cwd, "Git root lookup")
	if root_result.code ~= 0 or type(root_result.stdout) ~= "string" then
		fail("Git root lookup failed")
	end
	local root = vim.uv.fs_realpath(vim.trim(root_result.stdout))
	if not root then
		fail("Git root lookup returned an unavailable path")
	end
	local tracked =
		invoke(run, { "git", "ls-files", "--error-unmatch", "--", artifact }, root, "Git policy tracking check")
	if tracked.code == 1 then
		return {
			available = false,
			reason = "repository policy artifact is not Git-tracked",
			root = root,
			ref = artifact,
		}
	end
	if tracked.code ~= 0 then
		fail("Git policy tracking check failed")
	end
	local path = root .. "/" .. artifact
	if vim.fn.filereadable(path) ~= 1 then
		fail("Git-tracked repository policy artifact is unavailable")
	end
	local ok, document = pcall(vim.json.decode, table.concat(vim.fn.readfile(path), "\n"))
	if not ok or type(document) ~= "table" or (vim.islist(document) and next(document) ~= nil) then
		fail("repository policy artifact is not a JSON object")
	end
	for key in pairs(document) do
		if key ~= "enabled" and key ~= "rules" then
			fail("repository policy artifact contains unsupported field: " .. tostring(key))
		end
	end
	if document.enabled ~= true then
		return { available = false, reason = "repository policy artifact is not opted in", root = root, ref = artifact }
	end
	if type(document.rules) ~= "table" or (vim.islist(document.rules) and next(document.rules) ~= nil) then
		fail("repository policy artifact rules must be an object")
	end
	return {
		available = true,
		root = root,
		ref = artifact,
		policy = overlay.new({
			scope = "repository",
			target = root,
			rules = document.rules,
			provenance = { source = "repository-policy", ref = artifact },
		}),
	}
end

return M
