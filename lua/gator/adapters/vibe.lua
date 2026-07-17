local M = {}

local function fail(message)
	error("Gator Vibe adapter: " .. message, 3)
end

local function response(output)
	for line in vim.gsplit(output, "\n", { plain = true, trimempty = true }) do
		local ok, message = pcall(vim.json.decode, line)
		if ok and type(message) == "table" and message.id == 1 and type(message.result) == "table" then
			return message.result
		end
	end
end

local function capability(profile, name)
	return type(profile) == "table" and profile[name] == true
end

local function session_capability(profile, name)
	local sessions = type(profile) == "table" and profile.sessionCapabilities
	return type(sessions) == "table" and type(sessions[name]) == "table"
end

function M.probe(opts)
	opts = opts or {}
	if type(opts) ~= "table" or (opts.run ~= nil and type(opts.run) ~= "function") then
		fail("options must provide an optional run function")
	end
	for key in pairs(opts) do
		if key ~= "run" and key ~= "executable" and key ~= "acp_executable" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	local executable = opts.executable or "vibe"
	local acp_executable = opts.acp_executable or "vibe-acp"
	if type(executable) ~= "string" or executable == "" or type(acp_executable) ~= "string" or acp_executable == "" then
		fail("executables must be non-empty strings")
	end
	local invoke = opts.run
		or function(argv, input)
			local value = vim.system(argv, { text = true, stdin = input }):wait()
			return { code = value.code, stdout = value.stdout or "" }
		end
	local ok, version = pcall(invoke, { executable, "--version" })
	if not ok or type(version) ~= "table" or version.code ~= 0 or type(version.stdout) ~= "string" then
		return { provider = "vibe", available = false, reason = "Vibe executable or version is unavailable" }
	end
	local found = version.stdout:match("(%d+%.%d+%.%d+)")
	if not found then
		return { provider = "vibe", available = false, reason = "Vibe version output is unrecognized" }
	end
	local initialized = vim.json.encode({
		jsonrpc = "2.0",
		id = 1,
		method = "initialize",
		params = { protocolVersion = 1, clientCapabilities = vim.empty_dict() },
	}) .. "\n"
	local acp_ok, acp = pcall(invoke, { acp_executable }, initialized)
	local profile = acp_ok
		and type(acp) == "table"
		and acp.code == 0
		and type(acp.stdout) == "string"
		and response(acp.stdout)
	local agent = type(profile) == "table" and profile.agentCapabilities
	local prompt = type(agent) == "table" and agent.promptCapabilities
	local valid = type(profile) == "table" and profile.protocolVersion == 1 and type(agent) == "table"
	return {
		provider = "vibe",
		available = true,
		version = found,
		supported = valid,
		capabilities = {
			acp = valid,
			stdio = valid,
			load_session = valid and capability(agent, "loadSession"),
			session_list = valid and session_capability(agent, "list"),
			session_close = valid and session_capability(agent, "close"),
			session_fork = valid and session_capability(agent, "fork"),
			embedded_context = valid and capability(prompt, "embeddedContext"),
			images = valid and capability(prompt, "image"),
			structured_output = valid,
			plan = valid,
			approval = valid,
		},
		capability_error = valid and nil or "Vibe ACP initialization is unavailable",
	}
end

function M.auth(opts)
	if opts ~= nil and type(opts) ~= "table" then
		fail("authentication options must be a table")
	end
	if opts ~= nil and next(opts) ~= nil then
		fail("authentication options are unsupported")
	end
	return { provider = "vibe", authenticated = false, reason = "Vibe has no non-interactive credential status command" }
end

function M.launch(opts)
	if type(opts) ~= "table" or type(opts.manager) ~= "table" or type(opts.manager.launch) ~= "function" then
		fail("launch requires a process manager")
	end
	for key in pairs(opts) do
		if key ~= "manager" and key ~= "id" and key ~= "cwd" and key ~= "acp_executable" then
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
	local executable = opts.acp_executable or "vibe-acp"
	if type(executable) ~= "string" or executable == "" then
		fail("acp_executable must be a non-empty string")
	end
	return opts.manager:launch({ id = opts.id, command = { executable }, cwd = cwd })
end

return M
