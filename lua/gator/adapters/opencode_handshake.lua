local redact = require("gator.policy.redact")
local M = {}
local Handshake = {}

Handshake.__index = Handshake

local function fail(message)
	error("Gator OpenCode handshake: " .. redact.text(tostring(message)), 3)
end

local function string(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty string")
	end
	return value
end

local function object(value, allowed, name)
	if type(value) ~= "table" or vim.islist(value) then
		fail(name .. " must be an object")
	end
	for key in pairs(value) do
		if not allowed[key] then
			fail(name .. " contains unsupported field: " .. tostring(key))
		end
	end
	return value
end

local function reason(value, fallback)
	return type(value) == "string" and value ~= "" and redact.text(value) or fallback
end

local function enabled(value, name)
	if value ~= nil and type(value) ~= "boolean" then
		fail(name .. " must be a boolean")
	end
	return value == true
end

local function advertised(value, name)
	if value ~= nil and type(value) ~= "table" then
		fail(name .. " must be an object")
	end
	return value ~= nil
end

local function profile(value, id)
	object(value, { id = true, result = true, error = true }, "initialize response")
	if value.id ~= id then
		fail("initialize response does not match the handshake")
	end
	if value.error ~= nil then
		if value.result ~= nil then
			fail("initialize response cannot include result and error")
		end
		object(value.error, { code = true, message = true, data = true }, "initialize response error")
		if type(value.error.code) ~= "number" or value.error.code % 1 ~= 0 then
			fail("initialize response error.code must be an integer")
		end
		return nil,
			reason(string(value.error.message, "initialize response error.message"), "OpenCode initialize failed")
	end
	if value.result == nil then
		fail("initialize response must include result or error")
	end
	local result = object(
		value.result,
		{ protocolVersion = true, authMethods = true, agentInfo = true, agentCapabilities = true },
		"initialize result"
	)
	if result.protocolVersion ~= 1 then
		fail("initialize result.protocolVersion must be 1")
	end
	local info = object(result.agentInfo, { name = true, title = true, version = true }, "initialize agentInfo")
	local agent = object(
		result.agentCapabilities,
		{ loadSession = true, mcpCapabilities = true, promptCapabilities = true, sessionCapabilities = true },
		"initialize agentCapabilities"
	)
	local mcp = agent.mcpCapabilities
		and object(agent.mcpCapabilities, { http = true, sse = true }, "initialize mcpCapabilities")
	local prompt = agent.promptCapabilities
		and object(
			agent.promptCapabilities,
			{ image = true, audio = true, embeddedContext = true },
			"initialize promptCapabilities"
		)
	local sessions = agent.sessionCapabilities
		and object(
			agent.sessionCapabilities,
			{ close = true, fork = true, list = true, resume = true },
			"initialize sessionCapabilities"
		)
	return {
		agent = {
			name = string(info.name, "initialize agentInfo.name"),
			version = string(info.version, "initialize agentInfo.version"),
		},
		capabilities = {
			acp = true,
			stdio = true,
			load_session = enabled(agent.loadSession, "initialize agentCapabilities.loadSession"),
			session_list = sessions and advertised(sessions.list, "initialize sessionCapabilities.list") or false,
			session_close = sessions and advertised(sessions.close, "initialize sessionCapabilities.close") or false,
			session_fork = sessions and advertised(sessions.fork, "initialize sessionCapabilities.fork") or false,
			session_resume = sessions and advertised(sessions.resume, "initialize sessionCapabilities.resume") or false,
			mcp_http = mcp and enabled(mcp.http, "initialize mcpCapabilities.http") or false,
			mcp_sse = mcp and enabled(mcp.sse, "initialize mcpCapabilities.sse") or false,
			images = prompt and enabled(prompt.image, "initialize promptCapabilities.image") or false,
			embedded_context = prompt
					and enabled(prompt.embeddedContext, "initialize promptCapabilities.embeddedContext")
				or false,
		},
	}
end

local function status(value)
	return vim.deepcopy({
		provider = "opencode",
		state = value.state,
		reason = value.reason,
		agent = value.agent,
		capabilities = value.capabilities,
	})
end

function M.command(executable, cwd)
	return { string(executable or "opencode", "executable"), "acp", "--cwd", string(cwd or vim.fn.getcwd(), "cwd") }
end

function M.new(opts)
	if type(opts) ~= "table" or type(opts.request) ~= "function" then
		fail("new requires a request callback")
	end
	for key in pairs(opts) do
		if key ~= "request" and key ~= "id" and key ~= "client_info" and key ~= "executable" and key ~= "cwd" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	local id = opts.id or "gator-handshake"
	if type(id) ~= "string" and (type(id) ~= "number" or id % 1 ~= 0) then
		fail("id must be a string or integer")
	end
	local client = opts.client_info or { name = "gator", version = "1" }
	object(client, { name = true, title = true, version = true }, "client_info")
	string(client.name, "client_info.name")
	string(client.version, "client_info.version")
	if client.title ~= nil then
		string(client.title, "client_info.title")
	end
	return setmetatable({
		request = opts.request,
		id = id,
		client_info = vim.deepcopy(client),
		command = M.command(opts.executable, opts.cwd),
		state = "ready",
	}, Handshake)
end

function M.unavailable(value)
	return setmetatable(
		{ state = "unavailable", reason = reason(value, "OpenCode ACP transport is unavailable") },
		Handshake
	)
end

function M.is(value)
	return getmetatable(value) == Handshake
end

function Handshake:status()
	if not M.is(self) then
		fail("status requires an OpenCode handshake")
	end
	return status(self)
end

function Handshake:cancel(value)
	if not M.is(self) then
		fail("cancel requires an OpenCode handshake")
	end
	if self.state ~= "ready" then
		return false
	end
	self.state, self.reason = "cancelled", reason(value, "OpenCode handshake cancelled")
	return true
end

function Handshake:connect()
	if not M.is(self) then
		fail("connect requires an OpenCode handshake")
	end
	if self.state ~= "ready" then
		return status(self)
	end
	local ok, value = pcall(self.request, {
		id = self.id,
		method = "initialize",
		params = {
			protocolVersion = 1,
			clientCapabilities = vim.empty_dict(),
			clientInfo = vim.deepcopy(self.client_info),
		},
	})
	if not ok or type(value) ~= "table" then
		self.state = "failed"
		self.reason = redact.text(ok and "OpenCode initialize returned an invalid response" or tostring(value))
		return status(self)
	end
	local valid, result, detail = pcall(profile, value, self.id)
	if not valid then
		self.state, self.reason = "failed", redact.text(tostring(result))
		return status(self)
	end
	if result == nil then
		self.state, self.reason = "failed", detail
		return status(self)
	end
	self.agent, self.capabilities = result.agent, result.capabilities
	return status(self)
end

return M
