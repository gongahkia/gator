local M = {}
local Manager = {}

Manager.__index = Manager

M.default_timeout_ms = 300000
M.default_cancel_grace_ms = 1000
M.default_max_output_bytes = 65536

local manager_sequence = 0

local function fail(message)
	error("Gator run supervisor: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function positive_integer(value, name)
	if type(value) ~= "number" or value < 1 or value % 1 ~= 0 then
		fail(name .. " must be a positive integer")
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
	return vim.system(argv, {
		cwd = opts.cwd,
		text = true,
		timeout = opts.timeout_ms,
		stdout = function(_, data)
			if data then
				opts.stdout(data)
			end
		end,
		stderr = function(_, data)
			if data then
				opts.stderr(data)
			end
		end,
	}, callback)
end

local function default_timer()
	return vim.uv.new_timer()
end

local function capture(process, stream, value)
	if type(value) ~= "string" or value == "" then
		return
	end
	local current = process.output[stream]
	local remaining = process.max_output_bytes - #current
	if remaining <= 0 then
		process.output.truncated[stream] = true
		return
	end
	if #value > remaining then
		process.output[stream] = current .. value:sub(1, remaining)
		process.output.truncated[stream] = true
	else
		process.output[stream] = current .. value
	end
	local callback = process["on_" .. stream]
	if callback then
		callback(value)
	end
end

local function stop_timer(process)
	if not process.cancel_timer then
		return
	end
	pcall(process.cancel_timer.stop, process.cancel_timer)
	pcall(process.cancel_timer.close, process.cancel_timer)
	process.cancel_timer = nil
end

local function status(process)
	return vim.deepcopy({
		id = process.id,
		pid = process.pid,
		executable = process.command[1],
		timeout_ms = process.timeout_ms,
		state = process.state,
		result = process.result,
		failure = process.failure,
		output = process.output,
	})
end

local function notify_exit(process)
	if process.on_exit then
		process.on_exit(status(process))
	end
end

local function exit(process, result)
	if
		process.state == "completed"
		or process.state == "failed"
		or process.state == "cancelled"
		or process.state == "timed_out"
	then
		return
	end
	stop_timer(process)
	if type(result) ~= "table" then
		result = {}
	end
	if process.output.stdout == "" then
		capture(process, "stdout", result.stdout)
	end
	if process.output.stderr == "" then
		capture(process, "stderr", result.stderr)
	end
	local code = type(result.code) == "number" and result.code or 1
	local signal = type(result.signal) == "number" and result.signal or 0
	process.result = { code = code, signal = signal }
	if process.state == "cancelling" then
		process.state = "cancelled"
		process.failure = { kind = "cancelled", signal = signal }
	elseif code == 124 then
		process.state = "timed_out"
		process.failure = { kind = "timeout", timeout_ms = process.timeout_ms }
	elseif code == 0 then
		process.state = "completed"
	else
		process.state = "failed"
		process.failure = { kind = "exit", code = code, signal = signal }
	end
	notify_exit(process)
end

local function force_cancel(process)
	if process.state ~= "cancelling" then
		return
	end
	local ok, result = pcall(process.handle.kill, process.handle, 9)
	if not ok or result == false then
		process.failure = { kind = "cancel_failed" }
		process.state = "failed"
		stop_timer(process)
		notify_exit(process)
	end
end

local function launch_options(opts)
	for key in pairs(opts) do
		if
			key ~= "id"
			and key ~= "command"
			and key ~= "cwd"
			and key ~= "timeout_ms"
			and key ~= "cancel_grace_ms"
			and key ~= "max_output_bytes"
			and key ~= "on_stdout"
			and key ~= "on_stderr"
			and key ~= "on_exit"
		then
			fail("launch contains unsupported field: " .. tostring(key))
		end
	end
	if opts.cwd ~= nil and (type(opts.cwd) ~= "string" or opts.cwd == "") then
		fail("cwd must be a non-empty string")
	end
	for _, key in ipairs({ "on_stdout", "on_stderr", "on_exit" }) do
		if opts[key] ~= nil and type(opts[key]) ~= "function" then
			fail(key .. " must be a function")
		end
	end
	return {
		id = identifier(opts.id, "id"),
		command = command(opts.command),
		cwd = opts.cwd,
		timeout_ms = positive_integer(opts.timeout_ms or M.default_timeout_ms, "timeout_ms"),
		cancel_grace_ms = positive_integer(opts.cancel_grace_ms or M.default_cancel_grace_ms, "cancel_grace_ms"),
		max_output_bytes = positive_integer(opts.max_output_bytes or M.default_max_output_bytes, "max_output_bytes"),
		on_stdout = opts.on_stdout,
		on_stderr = opts.on_stderr,
		on_exit = opts.on_exit,
	}
end

function M.new(opts)
	opts = opts or {}
	if
		type(opts) ~= "table"
		or (opts.spawn ~= nil and type(opts.spawn) ~= "function")
		or (opts.timer ~= nil and type(opts.timer) ~= "function")
		or (opts.shutdown ~= nil and type(opts.shutdown) ~= "boolean")
	then
		fail("options must provide optional spawn, timer, and shutdown settings")
	end
	local manager = setmetatable(
		{ spawn = opts.spawn or default_spawn, timer = opts.timer or default_timer, processes = {} },
		Manager
	)
	if opts.shutdown ~= false then
		manager_sequence = manager_sequence + 1
		local group = vim.api.nvim_create_augroup("GatorRunSupervisor" .. manager_sequence, { clear = true })
		vim.api.nvim_create_autocmd("VimLeavePre", {
			group = group,
			once = true,
			callback = function()
				manager:shutdown()
			end,
		})
	end
	return manager
end

function Manager:launch(opts)
	if type(opts) ~= "table" then
		fail("launch requires options")
	end
	local value = launch_options(opts)
	if self.processes[value.id] then
		fail("process id is already managed: " .. value.id)
	end
	local process = {
		id = value.id,
		command = value.command,
		cwd = value.cwd,
		timeout_ms = value.timeout_ms,
		cancel_grace_ms = value.cancel_grace_ms,
		max_output_bytes = value.max_output_bytes,
		on_stdout = value.on_stdout,
		on_stderr = value.on_stderr,
		on_exit = value.on_exit,
		state = "starting",
		output = { stdout = "", stderr = "", truncated = { stdout = false, stderr = false } },
	}
	self.processes[value.id] = process
	local ok, handle = pcall(self.spawn, value.command, {
		cwd = value.cwd,
		timeout_ms = value.timeout_ms,
		stdout = function(chunk)
			capture(process, "stdout", chunk)
		end,
		stderr = function(chunk)
			capture(process, "stderr", chunk)
		end,
	}, function(result)
		exit(process, result)
	end)
	if not ok then
		process.state = "failed"
		process.failure = { kind = "spawn" }
		notify_exit(process)
		return status(process)
	end
	if type(handle) ~= "table" or type(handle.pid) ~= "number" or handle.pid < 1 or type(handle.kill) ~= "function" then
		self.processes[value.id] = nil
		fail("spawn must return a handle with pid and kill")
	end
	process.handle, process.pid = handle, handle.pid
	if process.state == "starting" then
		process.state = "running"
	end
	return status(process)
end

function Manager:status(id)
	id = identifier(id, "id")
	local process = self.processes[id]
	return process and status(process) or nil
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
	local ok, result = pcall(process.handle.kill, process.handle, 15)
	if not ok or result == false then
		fail("cancel failed")
	end
	process.state = "cancelling"
	local timer = self.timer()
	if
		timer == nil
		or type(timer.start) ~= "function"
		or type(timer.stop) ~= "function"
		or type(timer.close) ~= "function"
	then
		fail("timer must return start, stop, and close methods")
	end
	process.cancel_timer = timer
	timer:start(
		process.cancel_grace_ms,
		0,
		vim.schedule_wrap(function()
			force_cancel(process)
		end)
	)
	return status(process)
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
	local opts = {
		id = process.id,
		command = process.command,
		cwd = process.cwd,
		timeout_ms = process.timeout_ms,
		cancel_grace_ms = process.cancel_grace_ms,
		max_output_bytes = process.max_output_bytes,
		on_stdout = process.on_stdout,
		on_stderr = process.on_stderr,
		on_exit = process.on_exit,
	}
	self.processes[id] = nil
	return self:launch(opts)
end

function Manager:shutdown()
	local result = {}
	for id, process in pairs(self.processes) do
		if process.state == "running" then
			local ok, value = pcall(process.handle.kill, process.handle, 15)
			if ok and value ~= false then
				process.state = "cancelling"
				force_cancel(process)
			else
				process.state = "failed"
				process.failure = { kind = "shutdown_failed" }
				notify_exit(process)
			end
		end
		result[#result + 1] = status(process)
	end
	table.sort(result, function(left, right)
		return left.id < right.id
	end)
	return result
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
	stop_timer(process)
	self.processes[id] = nil
	return true
end

return M
