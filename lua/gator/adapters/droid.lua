local M = {}

local function fail(message)
	error("Gator Droid adapter: " .. message, 3)
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
	local executable = opts.executable or "droid"
	if type(executable) ~= "string" or executable == "" then
		fail("executable must be a non-empty string")
	end
	local invoke = opts.run
		or function(argv)
			local value = vim.system(argv, { text = true }):wait()
			return { code = value.code, stdout = value.stdout or "" }
		end
	local ok, version = pcall(invoke, { executable, "--version" })
	if not ok or type(version) ~= "table" or version.code ~= 0 or type(version.stdout) ~= "string" then
		return { provider = "droid", available = false, reason = "Droid executable or version is unavailable" }
	end
	local found = version.stdout:match("(%d+%.%d+%.%d+)")
	if not found then
		return { provider = "droid", available = false, reason = "Droid version output is unrecognized" }
	end
	local help_ok, help = pcall(invoke, { executable, "exec", "--help" })
	local output = help_ok
			and type(help) == "table"
			and help.code == 0
			and type(help.stdout) == "string"
			and help.stdout
		or ""
	local execute = output:find("--output-format", 1, true) ~= nil
	local jsonrpc = output:find("stream-jsonrpc", 1, true) ~= nil
	local session = output:find("--session-id", 1, true) ~= nil
	local fork = output:find("--fork", 1, true) ~= nil
	local cwd = output:find("--cwd", 1, true) ~= nil
	local supported = execute and jsonrpc and session and cwd
	return {
		provider = "droid",
		available = true,
		version = found,
		supported = supported,
		capabilities = {
			execute = execute,
			exec = execute,
			structured_output = execute,
			stream_jsonrpc = jsonrpc,
			session_resume = session,
			session_fork = fork,
			spec_mode = output:find("--use-spec", 1, true) ~= nil,
			autonomy = output:find("--auto", 1, true) ~= nil,
			tool_controls = output:find("--enabled-tools", 1, true) ~= nil
				and output:find("--disabled-tools", 1, true) ~= nil,
			cwd = cwd,
		},
		capability_error = not supported and "Droid exec does not expose the required documented JSON-RPC profile"
			or nil,
	}
end

function M.auth(opts)
	if opts ~= nil and type(opts) ~= "table" then
		fail("authentication options must be a table")
	end
	if opts ~= nil and next(opts) ~= nil then
		fail("authentication options are unsupported")
	end
	return {
		provider = "droid",
		authenticated = false,
		reason = "Droid has no non-interactive credential status command",
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
	local executable = opts.executable or "droid"
	if type(executable) ~= "string" or executable == "" then
		fail("executable must be a non-empty string")
	end
	return opts.manager:launch({
		id = opts.id,
		command = {
			executable,
			"exec",
			"--input-format",
			"stream-jsonrpc",
			"--output-format",
			"stream-jsonrpc",
			"--cwd",
			cwd,
		},
		cwd = cwd,
	})
end

return M
