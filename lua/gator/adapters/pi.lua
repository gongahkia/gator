local M = {}

M.range = { minimum = { 0, 80, 7 }, maximum = { 0, 80, 7 } }

local function fail(message)
	error("Gator Pi adapter: " .. message, 3)
end

local function version(value, name)
	if type(value) ~= "table" or #value ~= 3 then
		fail(name .. " must be a three-part version")
	end
	for index, part in ipairs(value) do
		if type(part) ~= "number" or part < 0 or part % 1 ~= 0 then
			fail(name .. " part " .. index .. " must be a non-negative integer")
		end
	end
	return value
end

local function compare(left, right)
	for index = 1, 3 do
		if left[index] ~= right[index] then
			return left[index] < right[index] and -1 or 1
		end
	end
	return 0
end

local function response(output)
	for line in vim.gsplit(output, "\n", { plain = true, trimempty = true }) do
		local ok, message = pcall(vim.json.decode, line)
		if
			ok
			and type(message) == "table"
			and message.id == "gator-probe"
			and message.type == "response"
			and message.command == "get_state"
			and message.success == true
			and type(message.data) == "table"
		then
			return message.data
		end
	end
end

function M.probe(opts)
	opts = opts or {}
	if type(opts) ~= "table" or (opts.run ~= nil and type(opts.run) ~= "function") then
		fail("options must provide an optional run function")
	end
	for key in pairs(opts) do
		if key ~= "run" and key ~= "executable" and key ~= "range" and key ~= "cwd" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	local executable = opts.executable or "pi"
	if type(executable) ~= "string" or executable == "" then
		fail("executable must be a non-empty string")
	end
	local range = opts.range or M.range
	if type(range) ~= "table" then
		fail("range must be a table")
	end
	local minimum, maximum = version(range.minimum, "range.minimum"), version(range.maximum, "range.maximum")
	if compare(minimum, maximum) > 0 then
		fail("range minimum cannot exceed maximum")
	end
	local cwd = opts.cwd or vim.fn.getcwd()
	if type(cwd) ~= "string" or cwd == "" then
		fail("cwd must be a non-empty string")
	end
	cwd = vim.uv.fs_realpath(cwd)
	if not cwd or vim.fn.isdirectory(cwd) ~= 1 then
		fail("cwd must resolve to a directory")
	end
	local invoke = opts.run
		or function(argv, input)
			local result = vim.system(argv, { cwd = cwd, text = true, stdin = input }):wait()
			return { code = result.code, stdout = result.stdout or "" }
		end
	local ok, result = pcall(invoke, { executable, "--version" })
	if not ok or type(result) ~= "table" or result.code ~= 0 or type(result.stdout) ~= "string" then
		return { provider = "pi", available = false, reason = "Pi executable or version is unavailable" }
	end
	local major, minor, patch = result.stdout:match("(%d+)%.(%d+)%.(%d+)")
	if not major then
		return { provider = "pi", available = false, reason = "Pi version output is unrecognized" }
	end
	local found = { tonumber(major), tonumber(minor), tonumber(patch) }
	local help_ok, help = pcall(invoke, { executable, "--help" })
	local output = help_ok
			and type(help) == "table"
			and help.code == 0
			and type(help.stdout) == "string"
			and help.stdout
		or ""
	local input = vim.json.encode({ type = "get_state", id = "gator-probe" }) .. "\n"
	local rpc_ok, rpc = pcall(invoke, {
		executable,
		"--mode",
		"rpc",
		"--no-session",
		"--no-tools",
		"--no-extensions",
		"--no-skills",
		"--no-context-files",
		"--offline",
	}, input)
	local state = rpc_ok
		and type(rpc) == "table"
		and rpc.code == 0
		and type(rpc.stdout) == "string"
		and response(rpc.stdout)
	local valid = type(state) == "table"
	return {
		provider = "pi",
		available = true,
		version = found,
		supported = compare(found, minimum) >= 0 and compare(found, maximum) <= 0,
		capabilities = {
			rpc = valid,
			stdio = valid,
			state = valid,
			sessions = output:find("--continue", 1, true) ~= nil
				and output:find("--resume", 1, true) ~= nil
				and output:find("--session", 1, true) ~= nil,
			tool_filters = output:find("--tools", 1, true) ~= nil and output:find("--exclude-tools", 1, true) ~= nil,
		},
		capability_error = valid and nil or "Pi RPC get_state is unavailable",
	}
end

function M.auth()
	return {
		provider = "pi",
		authenticated = false,
		reason = "Pi RPC does not expose a non-interactive credential-status contract",
	}
end

function M.launch(opts)
	if type(opts) ~= "table" or type(opts.manager) ~= "table" or type(opts.manager.launch) ~= "function" then
		fail("launch requires a process manager")
	end
	for key in pairs(opts) do
		if key ~= "manager" and key ~= "id" and key ~= "cwd" and key ~= "executable" then
			fail("launch contains unsupported field: " .. tostring(key))
		end
	end
	if type(opts.id) ~= "string" or not opts.id:match("^[a-z][a-z0-9_-]*$") then
		fail("id must be a lowercase identifier")
	end
	if type(opts.cwd) ~= "string" or opts.cwd == "" then
		fail("cwd must be a non-empty string")
	end
	local cwd = vim.uv.fs_realpath(opts.cwd)
	if not cwd or vim.fn.isdirectory(cwd) ~= 1 then
		fail("cwd must resolve to a directory")
	end
	local executable = opts.executable or "pi"
	if type(executable) ~= "string" or executable == "" then
		fail("executable must be a non-empty string")
	end
	return opts.manager:launch({ id = opts.id, command = { executable, "--mode", "rpc" }, cwd = cwd })
end

return M
