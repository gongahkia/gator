local git_boundary = require("gator.core.git")

local M = {}
local active = { starting = true, running = true, cancelling = true }

local function fail(message)
	error("Gator workspace monitor: " .. message, 3)
end

local function directory(value)
	if type(value) ~= "string" or value == "" then
		fail("root must be an existing directory")
	end
	local path = vim.uv.fs_realpath(value)
	if not path or vim.fn.isdirectory(path) ~= 1 then
		fail("root must be an existing directory")
	end
	return path
end

local function outcome(state, fields)
	fields = fields or {}
	fields.state = state
	return fields
end

local function cancelled(callback)
	if callback == nil then
		return false
	end
	local ok, value = pcall(callback)
	if not ok or type(value) ~= "boolean" then
		return nil
	end
	return value
end

local function invoke(git, argv, cwd, callback)
	local ok, result = pcall(git.run, git, argv, cwd, { cancelled = callback })
	if not ok or type(result) ~= "table" then
		return outcome("failed", { failure = "Git status failed" })
	end
	if result.state ~= "completed" or result.code ~= 0 or type(result.stdout) ~= "string" then
		return outcome(result.state or "failed", { failure = "Git status failed" })
	end
	return outcome("completed", { stdout = result.stdout })
end

local function agents(value, alive, callback)
	if type(value) ~= "table" or not vim.islist(value) then
		fail("agents must be an array")
	end
	local result, ids = {}, {}
	for index, agent in ipairs(value) do
		local stopped = cancelled(callback)
		if stopped == nil then
			return nil, outcome("failed", { failure = "cancellation check failed" })
		end
		if stopped then
			return nil, outcome("cancelled")
		end
		if type(agent) ~= "table" then
			fail("agent " .. index .. " must be a table")
		end
		for key in pairs(agent) do
			if key ~= "id" and key ~= "provider" and key ~= "session_id" and key ~= "pid" and key ~= "state" then
				fail("agent " .. index .. " contains unsupported field: " .. tostring(key))
			end
		end
		if type(agent.id) ~= "string" or not agent.id:match("^[a-z][a-z0-9_-]*$") then
			fail("agent " .. index .. " id must be a lowercase identifier")
		end
		if ids[agent.id] then
			fail("agent id is duplicated: " .. agent.id)
		end
		ids[agent.id] = true
		if
			type(agent.provider) ~= "string"
			or agent.provider == ""
			or type(agent.session_id) ~= "string"
			or agent.session_id == ""
		then
			fail("agent " .. index .. " must retain provider-owned session identity")
		end
		if
			type(agent.pid) ~= "number"
			or agent.pid % 1 ~= 0
			or agent.pid < 1
			or type(agent.state) ~= "string"
			or agent.state == ""
		then
			fail("agent " .. index .. " must provide pid and state")
		end
		local live = false
		if active[agent.state] then
			local ok, value = pcall(alive, agent.pid)
			if not ok or type(value) ~= "boolean" then
				return nil, outcome("failed", { failure = "agent liveness probe failed" })
			end
			live = value
		end
		result[index] = {
			id = agent.id,
			provider = agent.provider,
			session_id = agent.session_id,
			pid = agent.pid,
			state = agent.state,
			live = live,
			detached = active[agent.state] and not live,
		}
	end
	return result
end

local function changed_paths(output)
	local paths, seen = {}, {}
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
		if not seen[path] then
			seen[path] = true
			table.insert(paths, path)
		end
		if status:find("R", 1, true) or status:find("C", 1, true) then
			index = index + 1
			local previous = records[index]
			if not previous or previous == "" then
				fail("Git status returned an incomplete rename")
			end
			if not seen[previous] then
				seen[previous] = true
				table.insert(paths, previous)
			end
		end
		index = index + 1
	end
	table.sort(paths)
	return paths
end

function M.inspect(opts)
	if type(opts) ~= "table" then
		fail("inspect requires options")
	end
	for key in pairs(opts) do
		if
			key ~= "root"
			and key ~= "agents"
			and key ~= "run"
			and key ~= "git"
			and key ~= "alive"
			and key ~= "cancelled"
		then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	if opts.run ~= nil and type(opts.run) ~= "function" then
		fail("run must be a function")
	end
	if opts.git ~= nil and not git_boundary.is(opts.git) then
		fail("git must be a Git boundary")
	end
	if opts.run ~= nil and opts.git ~= nil then
		fail("inspect accepts either run or git")
	end
	if opts.alive ~= nil and type(opts.alive) ~= "function" then
		fail("alive must be a function")
	end
	if opts.cancelled ~= nil and type(opts.cancelled) ~= "function" then
		fail("cancelled must be a function")
	end
	local root = directory(opts.root)
	local stopped = cancelled(opts.cancelled)
	if stopped == nil then
		return outcome("failed", { root = root, failure = "cancellation check failed" })
	end
	if stopped then
		return outcome("cancelled", { root = root })
	end
	local git = opts.git or git_boundary.new({ run = opts.run })
	local status =
		invoke(git, { "git", "status", "--porcelain=v1", "-z", "--untracked-files=all" }, root, opts.cancelled)
	if status.state ~= "completed" then
		status.root = root
		return status
	end
	if opts.alive == nil and type(vim.uv.kill) ~= "function" then
		return outcome("unavailable", { root = root, failure = "process liveness probing is unavailable" })
	end
	local alive = opts.alive or function(pid)
		return vim.uv.kill(pid, 0) == true
	end
	local changed = changed_paths(status.stdout)
	local agent_values = opts.agents == nil and {} or opts.agents
	local agent_values, agent_state = agents(agent_values, alive, opts.cancelled)
	if not agent_values then
		agent_state.root = root
		return agent_state
	end
	return outcome("completed", {
		root = root,
		dirty = #changed > 0,
		changed_paths = changed,
		agents = agent_values,
	})
end

return M
