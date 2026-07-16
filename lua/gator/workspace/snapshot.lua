local M = {}

local function fail(message)
	error("Gator workspace snapshot: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function directory(value)
	if type(value) ~= "string" or value == "" then
		fail("cwd must be an existing directory")
	end
	local path = vim.uv.fs_realpath(value)
	if not path or vim.fn.isdirectory(path) ~= 1 then
		fail("cwd must be an existing directory")
	end
	return path
end

local function timestamp(value)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail("at must be a non-negative integer timestamp")
	end
	return value
end

local function invoke(run, argv, cwd, name)
	local ok, result = pcall(run, argv, cwd)
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

local function changes(output)
	local result = {}
	local records = vim.split(output, "\0", { plain = true, trimempty = true })
	local index = 1
	while index <= #records do
		local record = records[index]
		if #record < 4 then
			fail("Git status returned an invalid record")
		end
		local status, path = record:sub(1, 2), record:sub(4)
		if path == "" then
			fail("Git status returned an empty path")
		end
		table.insert(result, { path = path, status = status })
		if status:find("R", 1, true) or status:find("C", 1, true) then
			index = index + 1
			local previous = records[index]
			if not previous or previous == "" then
				fail("Git status returned an incomplete rename")
			end
			table.insert(result, { path = previous, status = status })
		end
		index = index + 1
	end
	table.sort(result, function(left, right)
		return left.path == right.path and left.status < right.status or left.path < right.path
	end)
	return result
end

function M.capture(opts)
	if type(opts) ~= "table" then
		fail("capture requires options")
	end
	for key in pairs(opts) do
		if key ~= "cwd" and key ~= "run_id" and key ~= "write" and key ~= "at" and key ~= "run" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	if opts.write ~= true then
		fail("snapshot capture requires a write-capable run")
	end
	if opts.run ~= nil and type(opts.run) ~= "function" then
		fail("run must be a function")
	end
	local cwd = directory(opts.cwd)
	local run = opts.run
		or function(argv, path)
			local result = vim.system(argv, { cwd = path, text = true }):wait()
			return { code = result.code, stdout = result.stdout or "" }
		end
	local root = directory(
		line(invoke(run, { "git", "rev-parse", "--show-toplevel" }, cwd, "Git root lookup"), "Git root lookup")
	)
	local base = line(
		invoke(run, { "git", "rev-parse", "--verify", "HEAD" }, root, "Git base revision lookup"),
		"Git base revision lookup"
	)
	local status = invoke(run, { "git", "status", "--porcelain=v1", "-z", "--untracked-files=all" }, root, "Git status")
	local digest = vim.fn.sha256(base .. "\0" .. status)
	local run_id = identifier(opts.run_id, "run_id")
	return {
		id = "git-snapshot-" .. vim.fn.sha256(run_id .. "\0" .. digest):sub(1, 24),
		run_id = run_id,
		type = "workspace.git_snapshot",
		at = timestamp(opts.at or os.time()),
		payload = {
			root = root,
			base_revision = base,
			working_tree = { dirty = status ~= "", digest = digest, changes = changes(status) },
		},
	}
end

return M
