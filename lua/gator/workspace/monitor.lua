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

local function invoke(run, argv, cwd)
	local ok, result = pcall(run, argv, cwd)
	if not ok or type(result) ~= "table" or result.code ~= 0 or type(result.stdout) ~= "string" then
		fail("Git status failed")
	end
	return result.stdout
end

local function agents(value, alive)
	if type(value) ~= "table" or not vim.islist(value) then
		fail("agents must be an array")
	end
	local result, ids = {}, {}
	for index, agent in ipairs(value) do
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
				fail("agent liveness probe failed")
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
		if key ~= "root" and key ~= "agents" and key ~= "run" and key ~= "alive" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	if opts.run ~= nil and type(opts.run) ~= "function" then
		fail("run must be a function")
	end
	if opts.alive ~= nil and type(opts.alive) ~= "function" then
		fail("alive must be a function")
	end
	local root = directory(opts.root)
	local run = opts.run
		or function(argv, cwd)
			local result = vim.system(argv, { cwd = cwd, text = true }):wait()
			return { code = result.code, stdout = result.stdout or "" }
		end
	local alive = opts.alive
		or function(pid)
			if type(vim.uv.kill) ~= "function" then
				fail("process liveness probing is unavailable")
			end
			return vim.uv.kill(pid, 0) == true
		end
	local changed =
		changed_paths(invoke(run, { "git", "status", "--porcelain=v1", "-z", "--untracked-files=all" }, root))
	local agent_values = opts.agents == nil and {} or opts.agents
	return {
		root = root,
		dirty = #changed > 0,
		changed_paths = changed,
		agents = agents(agent_values, alive),
	}
end

return M
