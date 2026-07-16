local M = {}
local prefix = ".gator/tasks/"

local function fail(message)
	error("Gator shared artifact sync: " .. message, 3)
end

local function reference(value, name)
	if type(value) ~= "string" or not value:match("^%.gator/tasks/[a-z][a-z0-9_-]*%.json$") then
		fail(name .. " must reference a shared task artifact")
	end
	return value
end

local function invoke(run, argv, cwd, name)
	local ok, result = pcall(run, argv, cwd)
	if not ok or type(result) ~= "table" or type(result.code) ~= "number" or result.code % 1 ~= 0 then
		fail(name .. " failed")
	end
	if result.stdout ~= nil and type(result.stdout) ~= "string" then
		fail(name .. " returned invalid stdout")
	end
	return { code = result.code, stdout = result.stdout or "" }
end

local function root(cwd, run)
	local result = invoke(run, { "git", "rev-parse", "--show-toplevel" }, cwd, "Git root lookup")
	if result.code ~= 0 then
		fail("Git root lookup failed")
	end
	local path = vim.uv.fs_realpath(vim.trim(result.stdout))
	if not path then
		fail("Git root lookup returned an unavailable path")
	end
	return path
end

local function snapshots(value)
	if type(value) ~= "table" or not vim.islist(value) then
		fail("snapshots must be a list")
	end
	local result, seen = {}, {}
	for index, snapshot in ipairs(value) do
		if type(snapshot) ~= "table" then
			fail("snapshots[" .. index .. "] must be an object")
		end
		for key in pairs(snapshot) do
			if key ~= "ref" and key ~= "digest" then
				fail("snapshots[" .. index .. "] contains unsupported field: " .. tostring(key))
			end
		end
		local ref = reference(snapshot.ref, "snapshots[" .. index .. "].ref")
		if seen[ref] then
			fail("snapshots must not duplicate refs")
		end
		if type(snapshot.digest) ~= "string" or not snapshot.digest:match("^[a-f0-9]+$") then
			fail("snapshots[" .. index .. "].digest must be a hex digest")
		end
		seen[ref] = true
		result[index] = { ref = ref, digest = snapshot.digest }
	end
	return result
end

local function digest(path)
	if vim.fn.filereadable(path) ~= 1 then
		return nil
	end
	return vim.fn.sha256(table.concat(vim.fn.readfile(path, "b"), "\n"))
end

local function changed(output)
	local result = {}
	for line in output:gmatch("[^\n]+") do
		if #line < 4 then
			fail("Git artifact status returned an invalid record")
		end
		local status, ref = line:sub(1, 2), line:sub(4)
		if ref:sub(1, #prefix) ~= prefix then
			fail("Git artifact status returned an unexpected path")
		end
		table.insert(result, { ref = ref, status = status })
	end
	table.sort(result, function(left, right)
		return left.ref < right.ref
	end)
	return result
end

local function conflicts(output)
	local result, seen = {}, {}
	for ref in output:gmatch("[^\n]+") do
		ref = reference(ref, "Git conflict path")
		if not seen[ref] then
			seen[ref] = true
			table.insert(result, ref)
		end
	end
	table.sort(result)
	return result
end

function M.capture(opts)
	if type(opts) ~= "table" then
		fail("capture requires options")
	end
	for key in pairs(opts) do
		if key ~= "cwd" and key ~= "refs" and key ~= "run" then
			fail("capture contains unsupported field: " .. tostring(key))
		end
	end
	if type(opts.cwd) ~= "string" or opts.cwd == "" then
		fail("cwd must be an existing directory")
	end
	if type(opts.refs) ~= "table" or not vim.islist(opts.refs) then
		fail("refs must be a list")
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
	local value = root(cwd, run)
	local result = {}
	for index, ref in ipairs(opts.refs) do
		ref = reference(ref, "refs[" .. index .. "]")
		local value_digest = digest(value .. "/" .. ref)
		if not value_digest then
			fail("shared artifact is unavailable: " .. ref)
		end
		result[index] = { ref = ref, digest = value_digest }
	end
	return result
end

function M.inspect(opts)
	if type(opts) ~= "table" then
		fail("inspect requires options")
	end
	for key in pairs(opts) do
		if key ~= "cwd" and key ~= "snapshots" and key ~= "run" then
			fail("inspect contains unsupported field: " .. tostring(key))
		end
	end
	if type(opts.cwd) ~= "string" or opts.cwd == "" then
		fail("cwd must be an existing directory")
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
	local value = root(cwd, run)
	local status = invoke(run, { "git", "status", "--porcelain=v1", "--", prefix }, value, "Git artifact status")
	if status.code ~= 0 then
		fail("Git artifact status failed")
	end
	local unmerged = invoke(
		run,
		{ "git", "diff", "--name-only", "--diff-filter=U", "--", prefix },
		value,
		"Git artifact conflict check"
	)
	if unmerged.code ~= 0 then
		fail("Git artifact conflict check failed")
	end
	local stale = {}
	for _, snapshot in ipairs(snapshots(opts.snapshots)) do
		local actual = digest(value .. "/" .. snapshot.ref)
		if not actual then
			table.insert(stale, { ref = snapshot.ref, reason = "shared artifact is missing locally" })
		elseif actual ~= snapshot.digest then
			table.insert(stale, { ref = snapshot.ref, reason = "shared artifact differs from recorded digest" })
		end
	end
	return { root = value, changes = changed(status.stdout), conflicts = conflicts(unmerged.stdout), stale = stale }
end

return M
