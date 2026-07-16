local M = {}

M.range = { minimum = { 1, 17, 15 }, maximum = { 1, 17, 15 } }

local function fail(message)
	error("Gator OpenCode probe: " .. message, 3)
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
		if key ~= "run" and key ~= "executable" and key ~= "range" and key ~= "cwd" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	local executable = opts.executable or "opencode"
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
			local result = vim.system(argv, { text = true, stdin = input }):wait()
			return { code = result.code, stdout = result.stdout or "" }
		end
	local ok, result = pcall(invoke, { executable, "--version" })
	if not ok or type(result) ~= "table" or result.code ~= 0 or type(result.stdout) ~= "string" then
		return { provider = "opencode", available = false, reason = "OpenCode executable or version is unavailable" }
	end
	local major, minor, patch = result.stdout:match("(%d+)%.(%d+)%.(%d+)")
	if not major then
		return { provider = "opencode", available = false, reason = "OpenCode version output is unrecognized" }
	end
	local found = { tonumber(major), tonumber(minor), tonumber(patch) }
	local initialized = vim.json.encode({
		jsonrpc = "2.0",
		id = 1,
		method = "initialize",
		params = { protocolVersion = 1, clientCapabilities = vim.empty_dict() },
	}) .. "\n"
	local acp_ok, acp = pcall(invoke, { executable, "acp", "--cwd", cwd }, initialized)
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
		provider = "opencode",
		available = true,
		version = found,
		supported = compare(found, minimum) >= 0 and compare(found, maximum) <= 0,
		capabilities = {
			acp = valid,
			stdio = valid,
			load_session = valid and capability(agent, "loadSession"),
			session_list = valid and session_capability(agent, "list"),
			session_close = valid and session_capability(agent, "close"),
			session_fork = valid and session_capability(agent, "fork"),
			mcp_http = valid and capability(mcp, "http"),
			mcp_sse = valid and capability(mcp, "sse"),
			embedded_context = valid and capability(prompt, "embeddedContext"),
			images = valid and capability(prompt, "image"),
		},
		capability_error = valid and nil or "OpenCode ACP initialization is unavailable",
	}
end

return M
