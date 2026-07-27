local M = {}
local Cleanup = {}

Cleanup.__index = Cleanup

local function fail(message)
	error("Gator workspace cleanup: " .. message, 3)
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

local function invoke(run, argv, cwd, name, output)
	local ok, result = pcall(run, argv, cwd)
	if not ok or type(result) ~= "table" or result.code ~= 0 then
		fail(name .. " failed")
	end
	if output and type(result.stdout) ~= "string" then
		fail(name .. " returned no output")
	end
	return result.stdout
end

local function worktrees(output)
	local result = {}
	for _, block in ipairs(vim.split(output, "\n\n", { plain = true, trimempty = true })) do
		local value = {}
		for line in block:gmatch("[^\n]+") do
			if line:sub(1, 9) == "worktree " then
				value.path = line:sub(10)
			elseif line:sub(1, 7) == "branch " then
				value.branch = line:sub(8)
			elseif line:sub(1, 7) == "locked " or line == "locked" then
				value.locked = true
			elseif line:sub(1, 9) == "prunable " or line == "prunable" then
				value.prunable = true
			end
		end
		if type(value.path) ~= "string" or value.path == "" then
			fail("Git worktree list returned an invalid record")
		end
		table.insert(result, value)
	end
	return result
end

local function same_plan(left, right)
	if #left ~= #right then
		return false
	end
	for index, candidate in ipairs(left) do
		if candidate.path ~= right[index].path then
			return false
		end
	end
	return true
end

function M.new(opts)
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	for key in pairs(opts) do
		if key ~= "root" and key ~= "run" and key ~= "active" and key ~= "owned" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	if opts.run ~= nil and type(opts.run) ~= "function" then
		fail("run must be a function")
	end
	if type(opts.active) ~= "function" then
		fail("active must be a function")
	end
	if opts.owned ~= nil and type(opts.owned) ~= "function" then
		fail("owned must be a function")
	end
	local root = directory(opts.root, "root")
	local run = opts.run
		or function(argv, cwd)
			local result = vim.system(argv, { cwd = cwd, text = true }):wait()
			return { code = result.code, stdout = result.stdout or "" }
		end
	return setmetatable(
		{ root = root, run = run, active = opts.active, owned = opts.owned, plans = setmetatable({}, { __mode = "k" }) },
		Cleanup
	)
end

function Cleanup:plan()
	local output = invoke(self.run, { "git", "worktree", "list", "--porcelain" }, self.root, "Git worktree list", true)
	local result = {}
	for _, value in ipairs(worktrees(output)) do
		local path = vim.uv.fs_realpath(value.path)
		if path and path ~= self.root and not value.locked and not value.prunable then
			if self.owned then
				local ok, owned = pcall(self.owned, path)
				if not ok or type(owned) ~= "boolean" then
					fail("worktree ownership probe failed")
				end
				if not owned then
					goto continue
				end
			end
			local ok, active = pcall(self.active, path)
			if not ok or type(active) ~= "boolean" then
				fail("worktree activity probe failed")
			end
			local status = invoke(
				self.run,
				{ "git", "status", "--porcelain=v1", "-z", "--untracked-files=all" },
				path,
				"Git worktree status",
				true
			)
			if not active and status == "" then
				table.insert(result, { path = path, branch = value.branch })
			end
		end
		::continue::
	end
	table.sort(result, function(left, right)
		return left.path < right.path
	end)
	self.plans[result] = true
	return result
end

function Cleanup:prune(plan, confirm)
	if confirm ~= true then
		fail("cleanup requires explicit confirmation")
	end
	if type(plan) ~= "table" or not self.plans[plan] then
		fail("plan must be returned by cleanup:plan")
	end
	local current = self:plan()
	if not same_plan(plan, current) then
		fail("cleanup plan is stale")
	end
	local removed = {}
	for _, candidate in ipairs(plan) do
		local ok, active = pcall(self.active, candidate.path)
		if not ok or type(active) ~= "boolean" then
			fail("worktree activity probe failed")
		end
		if active then
			fail("cleanup plan is stale")
		end
		local status = invoke(
			self.run,
			{ "git", "status", "--porcelain=v1", "-z", "--untracked-files=all" },
			candidate.path,
			"Git worktree status",
			true
		)
		if status ~= "" then
			fail("cleanup plan is stale")
		end
		invoke(self.run, { "git", "worktree", "remove", candidate.path }, self.root, "Git worktree removal", false)
		table.insert(removed, candidate.path)
	end
	self.plans[plan] = nil
	return removed
end

return M
