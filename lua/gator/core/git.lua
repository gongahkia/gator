local redact = require("gator.policy.redact")

local M = {}
local Git = {}

Git.__index = Git
M.states = { completed = true, failed = true, unavailable = true, cancelled = true }

local function fail(message)
	error("Gator Git: " .. redact.text(tostring(message)), 3)
end

local function command(value)
	if type(value) ~= "table" or not vim.islist(value) or #value == 0 or value[1] ~= "git" then
		fail("command must be a non-empty Git argument list")
	end
	for index, argument in ipairs(value) do
		if type(argument) ~= "string" or argument == "" or argument:find("\0", 1, true) then
			fail("command argument " .. index .. " must be non-empty text without NUL")
		end
	end
	return vim.deepcopy(value)
end

local function directory(value)
	if type(value) ~= "string" or value == "" or value:find("\0", 1, true) then
		fail("cwd must be non-empty text without NUL")
	end
	return value
end

local function cancelled(callback)
	if callback == nil then
		return false
	end
	local ok, value = pcall(callback)
	if not ok then
		fail("cancellation check failed: " .. tostring(value))
	end
	if type(value) ~= "boolean" then
		fail("cancellation check must return boolean")
	end
	return value
end

local function unavailable(detail)
	return { state = "unavailable", code = nil, stdout = "", stderr = redact.text(tostring(detail)) }
end

local function cancellation()
	return { state = "cancelled", code = nil, stdout = "", stderr = "" }
end

local function result(value)
	if type(value) ~= "table" then
		fail("runner must return a result table")
	end
	if value.state == "unavailable" then
		return unavailable(value.stderr or "Git is unavailable")
	end
	if value.state == "cancelled" then
		return cancellation()
	end
	if value.state ~= nil then
		fail("runner returned unsupported state")
	end
	if type(value.code) ~= "number" or value.code < 0 or value.code % 1 ~= 0 then
		fail("runner result code must be a non-negative integer")
	end
	if type(value.stdout) ~= "string" or (value.stderr ~= nil and type(value.stderr) ~= "string") then
		fail("runner result output must be text")
	end
	return {
		state = value.code == 0 and "completed" or "failed",
		code = value.code,
		stdout = value.stdout,
		stderr = redact.text(value.stderr or ""),
	}
end

local function default_run(argv, cwd)
	local value = vim.system(argv, { cwd = cwd, text = true }):wait()
	return { code = value.code, stdout = value.stdout or "", stderr = value.stderr or "" }
end

function M.new(opts)
	if opts == nil then
		opts = {}
	end
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	for key in pairs(opts) do
		if key ~= "run" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	if opts.run ~= nil and type(opts.run) ~= "function" then
		fail("run must be a function")
	end
	return setmetatable({ run_command = opts.run or default_run }, Git)
end

function M.is(value)
	return getmetatable(value) == Git
end

function Git:run(argv, cwd, opts)
	if not M.is(self) then
		fail("run requires a Git boundary")
	end
	argv, cwd = command(argv), directory(cwd)
	if opts == nil then
		opts = {}
	end
	if type(opts) ~= "table" then
		fail("run options must be a table")
	end
	for key in pairs(opts) do
		if key ~= "cancelled" then
			fail("run options contain unsupported field: " .. tostring(key))
		end
	end
	if opts.cancelled ~= nil and type(opts.cancelled) ~= "function" then
		fail("cancelled must be a function")
	end
	if cancelled(opts.cancelled) then
		return cancellation()
	end
	local ok, value = pcall(self.run_command, argv, cwd)
	if not ok then
		return unavailable(value)
	end
	if cancelled(opts.cancelled) then
		return cancellation()
	end
	return result(value)
end

return M
