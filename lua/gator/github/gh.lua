local M = {}

local function fail(message)
	error("Gator GitHub CLI: " .. message, 3)
end

local function invoke(run, argv, cwd, name)
	local ok, result = pcall(run, argv, cwd)
	if not ok or type(result) ~= "table" or type(result.code) ~= "number" or result.code % 1 ~= 0 then
		fail(name .. " failed")
	end
	if result.stdout ~= nil and type(result.stdout) ~= "string" then
		fail(name .. " returned invalid stdout")
	end
	if result.stderr ~= nil and type(result.stderr) ~= "string" then
		fail(name .. " returned invalid stderr")
	end
	return { code = result.code, stdout = result.stdout or "", stderr = result.stderr or "" }
end

local function capability(authenticated, result, name)
	if not authenticated then
		return { available = false, reason = "GitHub CLI authentication is unavailable" }
	end
	if result.code ~= 0 then
		return { available = false, reason = "GitHub CLI " .. name .. " capability is unavailable" }
	end
	return { available = true }
end

function M.detect(opts)
	if opts == nil then
		opts = {}
	end
	if type(opts) ~= "table" then
		fail("detect requires options")
	end
	for key in pairs(opts) do
		if key ~= "cwd" and key ~= "run" then
			fail("detect contains unsupported field: " .. tostring(key))
		end
	end
	if opts.cwd ~= nil and (type(opts.cwd) ~= "string" or opts.cwd == "") then
		fail("cwd must be a non-empty string")
	end
	if opts.run ~= nil and type(opts.run) ~= "function" then
		fail("run must be a function")
	end
	local run = opts.run
		or function(argv, cwd)
			local result = vim.system(argv, { cwd = cwd, text = true }):wait()
			return { code = result.code, stdout = result.stdout or "", stderr = result.stderr or "" }
		end
	local version = invoke(run, { "gh", "--version" }, opts.cwd, "GitHub CLI version check")
	if version.code == 127 then
		return { available = false, reason = "GitHub CLI is unavailable" }
	end
	if version.code ~= 0 then
		fail("GitHub CLI version check failed")
	end
	local parsed = (version.stdout .. "\n" .. version.stderr):match("gh version (%d+%.%d+%.%d+)")
	if not parsed then
		fail("GitHub CLI version is unavailable")
	end
	local auth =
		invoke(run, { "gh", "auth", "status", "--hostname", "github.com" }, opts.cwd, "GitHub CLI authentication check")
	local authenticated = auth.code == 0
	local issue = invoke(run, { "gh", "issue", "--help" }, opts.cwd, "GitHub issue capability check")
	local pull_request = invoke(run, { "gh", "pr", "--help" }, opts.cwd, "GitHub pull request capability check")
	return {
		available = true,
		version = parsed,
		authenticated = authenticated,
		capabilities = {
			issue_import = capability(authenticated, issue, "issue import"),
			pull_request_import = capability(authenticated, pull_request, "pull request import"),
		},
	}
end

return M
