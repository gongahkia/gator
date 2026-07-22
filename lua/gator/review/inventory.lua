local M = {}

local function fail(message)
	error("Gator review inventory: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function workspace(value)
	if type(value) ~= "table" then
		fail("workspace must be a table")
	end
	for key in pairs(value) do
		if key ~= "id" and key ~= "kind" and key ~= "root" then
			fail("workspace contains unsupported field: " .. tostring(key))
		end
	end
	if value.id ~= nil then
		identifier(value.id, "workspace.id")
	end
	if value.kind ~= "project" and value.kind ~= "worktree" then
		fail("workspace.kind must be project or worktree")
	end
	if type(value.root) ~= "string" or value.root == "" then
		fail("workspace.root must be an existing directory")
	end
	local root = vim.uv.fs_realpath(value.root)
	if not root or vim.fn.isdirectory(root) ~= 1 then
		fail("workspace.root must be an existing directory")
	end
	return { id = value.id, kind = value.kind, root = root }
end

local function invoke(run, argv, cwd)
	local ok, result = pcall(run, argv, cwd)
	if not ok or type(result) ~= "table" or result.code ~= 0 or type(result.stdout) ~= "string" then
		fail("Git status failed")
	end
	return result.stdout
end

local function category(status)
	if status == "??" then
		return "untracked"
	end
	if status:find("U", 1, true) then
		return "unmerged"
	end
	if status:find("R", 1, true) then
		return "renamed"
	end
	if status:find("C", 1, true) then
		return "copied"
	end
	if status:find("A", 1, true) then
		return "added"
	end
	if status:find("D", 1, true) then
		return "deleted"
	end
	if status:find("M", 1, true) then
		return "modified"
	end
	if status:find("T", 1, true) then
		return "type_changed"
	end
	fail("Git status returned an unsupported status: " .. status)
end

local function files(output)
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
		local file = { path = path, status = status, category = category(status) }
		if status:find("R", 1, true) or status:find("C", 1, true) then
			index = index + 1
			file.previous_path = records[index]
			if not file.previous_path or file.previous_path == "" then
				fail("Git status returned an incomplete rename")
			end
		end
		table.insert(result, file)
		index = index + 1
	end
	table.sort(result, function(left, right)
		return left.path < right.path
	end)
	return result
end

local function hunks(task_id, path, output)
	local result = {}
	for header in output:gmatch("[^\n]+") do
		local before_start, before_count, after_start, after_count =
			header:match("^@@ %-(%d+),?(%d*) %+(%d+),?(%d*) @@")
		if before_start then
			before_count = before_count == "" and 1 or tonumber(before_count)
			after_count = after_count == "" and 1 or tonumber(after_count)
			table.insert(result, {
				id = "hunk-" .. vim.fn.sha256(task_id .. "\0" .. path .. "\0" .. header):sub(1, 16),
				before_start = tonumber(before_start),
				before_count = before_count,
				after_start = tonumber(after_start),
				after_count = after_count,
			})
		end
	end
	return result
end

function M.build(opts)
	if type(opts) ~= "table" then
		fail("build requires options")
	end
	for key in pairs(opts) do
		if key ~= "task_id" and key ~= "workspace" and key ~= "run" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	if opts.run ~= nil and type(opts.run) ~= "function" then
		fail("run must be a function")
	end
	local task_id, value = identifier(opts.task_id, "task_id"), workspace(opts.workspace)
	local run = opts.run
		or function(argv, cwd)
			local result = vim.system(argv, { cwd = cwd, text = true }):wait()
			return { code = result.code, stdout = result.stdout or "" }
		end
	local inventory =
		files(invoke(run, { "git", "status", "--porcelain=v1", "-z", "--untracked-files=all" }, value.root))
	for _, file in ipairs(inventory) do
		file.hunks = file.category == "untracked" and {}
			or hunks(
				task_id,
				file.path,
				invoke(run, { "git", "diff", "--no-ext-diff", "--unified=0", "HEAD", "--", file.path }, value.root)
			)
	end
	return { task_id = task_id, workspace = value, files = inventory }
end

return M
