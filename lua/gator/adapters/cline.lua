local M = {}

local function fail(message)
	error("Gator Cline probe: " .. message, 3)
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

function M.probe(opts)
	opts = opts or {}
	if type(opts) ~= "table" or (opts.run ~= nil and type(opts.run) ~= "function") then
		fail("options must provide an optional run function")
	end
	for key in pairs(opts) do
		if key ~= "run" and key ~= "executable" and key ~= "cwd" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	local command = executable(opts.executable or "cline")
	local directory = cwd(opts.cwd or vim.fn.getcwd())
	local invoke = opts.run
		or function(argv, input)
			local value = vim.system(argv, { text = true, stdin = input }):wait()
			return { code = value.code, stdout = value.stdout or "" }
		end
	local ok, version = pcall(invoke, { command, "--version" })
	if not ok or type(version) ~= "table" or version.code ~= 0 or type(version.stdout) ~= "string" then
		return { provider = "cline", available = false, reason = "Cline executable or version is unavailable" }
	end
	local found = version.stdout:match("(%d+%.%d+%.%d+)")
	if not found then
		return { provider = "cline", available = false, reason = "Cline version output is unrecognized" }
	end
	local help_ok, help = pcall(invoke, { command, "--help" })
	local output = help_ok
			and type(help) == "table"
			and help.code == 0
			and type(help.stdout) == "string"
			and help.stdout
		or ""
	local initialized = vim.json.encode({
		jsonrpc = "2.0",
		id = 1,
		method = "initialize",
		params = { protocolVersion = 1, clientCapabilities = vim.empty_dict() },
	}) .. "\n"
	local acp_ok, acp = pcall(invoke, { command, "--acp", "--cwd", directory }, initialized)
	local profile = acp_ok
		and type(acp) == "table"
		and acp.code == 0
		and type(acp.stdout) == "string"
		and response(acp.stdout)
	local agent = type(profile) == "table" and profile.agentCapabilities
	local mcp = type(agent) == "table" and agent.mcpCapabilities
	local prompt = type(agent) == "table" and agent.promptCapabilities
	local valid = type(profile) == "table" and profile.protocolVersion == 1 and type(agent) == "table"
	return {
		provider = "cline",
		available = true,
		version = found,
		supported = true,
		capabilities = {
			cli = true,
			acp = valid,
			stdio = valid,
			structured_output = output:find("--json", 1, true) ~= nil,
			session_create = valid,
			session_history = output:find("history", 1, true) ~= nil,
			session_resume = output:find("--id", 1, true) ~= nil,
			plan = output:find("--plan", 1, true) ~= nil,
			auto_approve = output:find("--auto%-approve") ~= nil,
			acp_load_session = valid and capability(agent, "loadSession"),
			mcp_http = valid and capability(mcp, "http"),
			mcp_sse = valid and capability(mcp, "sse"),
			embedded_context = valid and capability(prompt, "embeddedContext"),
			images = valid and capability(prompt, "image"),
		},
		capability_error = valid and nil or "Cline ACP initialization is unavailable",
	}
end

function M.auth()
	return {
		provider = "cline",
		authenticated = false,
		reason = "Cline does not document a provider-independent authentication-status contract",
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
	local directory = cwd(opts.cwd)
	return opts.manager:launch({
		id = opts.id,
		command = { executable(opts.executable or "cline"), "--acp" },
		cwd = directory,
	})
end

return M
