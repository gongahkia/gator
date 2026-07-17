local M = {}
local Manager = {}

Manager.__index = Manager

local function fail(message)
	error("Gator adapter process: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function command(value)
	if type(value) ~= "table" or not vim.islist(value) or #value == 0 then
		fail("command must be a non-empty argv array")
	end
	local result = {}
	for index, argument in ipairs(value) do
		if type(argument) ~= "string" or argument == "" then
			fail("command argument " .. index .. " must be a non-empty string")
		end
		result[index] = argument
	end
	return result
end

local function default_spawn(argv, opts, callback)
	return vim.system(argv, { cwd = opts.cwd, text = true, timeout = opts.timeout_ms }, callback)
end

function M.new(opts)
	opts = opts or {}
	if type(opts) ~= "table" or (opts.spawn ~= nil and type(opts.spawn) ~= "function") then
		fail("options must provide an optional spawn function")
	end
	return setmetatable({ spawn = opts.spawn or default_spawn, processes = {} }, Manager)
end

function Manager:launch(opts)
	if type(opts) ~= "table" then
		fail("launch requires options")
	end
	for key in pairs(opts) do
		if key ~= "id" and key ~= "command" and key ~= "cwd" and key ~= "timeout_ms" then
			fail("launch contains unsupported field: " .. tostring(key))
		end
	end
	local id = identifier(opts.id, "id")
	if self.processes[id] then
		fail("process id is already managed: " .. id)
	end
	if opts.cwd ~= nil and (type(opts.cwd) ~= "string" or opts.cwd == "") then
		fail("cwd must be a non-empty string")
	end
	if
		opts.timeout_ms ~= nil
		and (type(opts.timeout_ms) ~= "number" or opts.timeout_ms < 1 or opts.timeout_ms % 1 ~= 0)
	then
		fail("timeout_ms must be a positive integer")
	end
	local argv = command(opts.command)
	local process = { id = id, command = argv, cwd = opts.cwd, timeout_ms = opts.timeout_ms, state = "starting" }
	self.processes[id] = process
	local ok, handle = pcall(self.spawn, argv, { cwd = opts.cwd, timeout_ms = opts.timeout_ms }, function(result)
		process.result = { code = result.code, signal = result.signal }
		if process.state == "cancelling" then
			process.state = "cancelled"
		else
			process.state = result.code == 0 and "completed" or "failed"
		end
	end)
	if not ok then
		self.processes[id] = nil
		fail("launch failed: " .. handle)
	end
	if type(handle) ~= "table" or type(handle.pid) ~= "number" or handle.pid < 1 or type(handle.kill) ~= "function" then
		self.processes[id] = nil
		fail("spawn must return a handle with pid and kill")
	end
	process.handle, process.pid, process.state = handle, handle.pid, "running"
	return self:status(id)
end

function Manager:status(id)
	id = identifier(id, "id")
	local process = self.processes[id]
	if not process then
		return nil
	end
	return vim.deepcopy({
		id = process.id,
		pid = process.pid,
		executable = process.command[1],
		timeout_ms = process.timeout_ms,
		state = process.state,
		result = process.result,
	})
end

function Manager:cancel(id)
	id = identifier(id, "id")
	local process = self.processes[id]
	if not process then
		fail("process is not managed: " .. id)
	end
	if process.state ~= "running" then
		fail("only running processes can be cancelled")
	end
	local ok, err = pcall(process.handle.kill, process.handle, 15)
	if not ok or err == false then
		fail("cancel failed")
	end
	process.state = "cancelling"
	return self:status(id)
end

function Manager:restart(id)
	id = identifier(id, "id")
	local process = self.processes[id]
	if not process then
		fail("process is not managed: " .. id)
	end
	if process.state == "running" or process.state == "starting" or process.state == "cancelling" then
		fail("restart requires a terminal process state")
	end
	local argv, cwd, timeout_ms = process.command, process.cwd, process.timeout_ms
	self.processes[id] = nil
	return self:launch({ id = id, command = argv, cwd = cwd, timeout_ms = timeout_ms })
end

function Manager:cleanup(id)
	id = identifier(id, "id")
	local process = self.processes[id]
	if not process then
		return false
	end
	if process.state == "running" or process.state == "starting" or process.state == "cancelling" then
		fail("cleanup requires a terminal process state")
	end
	self.processes[id] = nil
	return true
end

return M
