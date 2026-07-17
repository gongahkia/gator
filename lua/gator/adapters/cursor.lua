local M = {}

local function fail(message)
	error("Gator Cursor probe: " .. message, 3)
end

local function run(argv)
	local value = vim.system(argv, { text = true }):wait()
	return { code = value.code, stdout = value.stdout or "" }
end

local function executable(value)
	if type(value) ~= "string" or value == "" then
		fail("executable must be a non-empty string")
	end
	return value
end

local function cwd(value)
	if type(value) ~= "string" or value == "" then
		fail("cwd must be a non-empty string")
	end
	value = vim.uv.fs_realpath(value)
	if not value or vim.fn.isdirectory(value) ~= 1 then
		fail("cwd must resolve to a directory")
	end
	return value
end

function M.probe(opts)
	opts = opts or {}
	if type(opts) ~= "table" or (opts.run ~= nil and type(opts.run) ~= "function") then
		fail("options must provide an optional run function")
	end
	for key in pairs(opts) do
		if key ~= "run" and key ~= "executable" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	local executable = executable(opts.executable or "cursor-agent")
	local invoke = opts.run or run
	local ok, version = pcall(invoke, { executable, "--version" })
	if not ok or type(version) ~= "table" or version.code ~= 0 or type(version.stdout) ~= "string" then
		return { provider = "cursor", available = false, reason = "Cursor executable or version is unavailable" }
	end
	local found = version.stdout:match("(%d+%.%d+%.%d+)")
	if not found then
		return { provider = "cursor", available = false, reason = "Cursor version output is unrecognized" }
	end
	local help_ok, help = pcall(invoke, { executable, "--help" })
	local output = help_ok
			and type(help) == "table"
			and help.code == 0
			and type(help.stdout) == "string"
			and help.stdout
		or ""
	return {
		provider = "cursor",
		available = true,
		version = found,
		supported = true,
		capabilities = {
			cli = true,
			print = output:find("--print", 1, true) ~= nil or output:find("-p", 1, true) ~= nil,
			structured_output = output:find("stream%-json") ~= nil and output:find("--output%-format") ~= nil,
			session_history = output:find("ls", 1, true) ~= nil,
			session_resume = output:find("--resume", 1, true) ~= nil,
			force = output:find("--force", 1, true) ~= nil,
			auth_status = output:find("status", 1, true) ~= nil,
		},
	}
end

function M.auth()
	return {
		provider = "cursor",
		authenticated = false,
		reason = "Cursor status does not document a machine-readable authentication-status schema",
	}
end

function M.launch(opts)
	if type(opts) ~= "table" or type(opts.manager) ~= "table" or type(opts.manager.launch) ~= "function" then
		fail("launch requires a process manager")
	end
	for key in pairs(opts) do
		if key ~= "manager" and key ~= "id" and key ~= "cwd" and key ~= "args" and key ~= "executable" then
			fail("launch contains unsupported field: " .. tostring(key))
		end
	end
	if type(opts.id) ~= "string" or not opts.id:match("^[a-z][a-z0-9_-]*$") then
		fail("id must be a lowercase identifier")
	end
	local args = opts.args or {}
	if type(args) ~= "table" or not vim.islist(args) then
		fail("args must be an array")
	end
	local command = { executable(opts.executable or "cursor-agent") }
	for index, argument in ipairs(args) do
		if type(argument) ~= "string" or argument == "" then
			fail("argument " .. index .. " must be a non-empty string")
		end
		table.insert(command, argument)
	end
	return opts.manager:launch({ id = opts.id, command = command, cwd = cwd(opts.cwd) })
end

return M
