local redact = require("gator.policy.redact")
local M = {}
local Handshake = {}

Handshake.__index = Handshake

local function fail(message)
	error("Gator Gemini handshake: " .. redact.text(tostring(message)), 3)
end

local function string(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty string")
	end
	return value
end

local function fields(value, allowed, name)
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

local function capability(value, key)
	return type(value) == "table" and value[key] == true
end

local function profile(value, id)
	if value.error ~= nil then
		fields(value, { id = true, error = true }, "initialize response")
		if value.id ~= id then
			fail("initialize response does not match the handshake")
		end
		fields(value.error, { code = true, message = true, data = true }, "initialize response error")
		if type(value.error.code) ~= "number" or value.error.code % 1 ~= 0 then
			fail("initialize response error.code must be an integer")
		end
		return nil, reason(string(value.error.message, "initialize response error.message"), "Gemini initialize failed")
	end
	fields(value, { id = true, result = true }, "initialize response")
	if value.id ~= id then
		fail("initialize response does not match the handshake")
	end
	fields(
		value.result,
		{ protocolVersion = true, authMethods = true, agentInfo = true, agentCapabilities = true },
		"initialize result"
	)
	if value.result.protocolVersion ~= 1 then
		fail("initialize result.protocolVersion must be 1")
	end
	local info = fields(value.result.agentInfo, { name = true, title = true, version = true }, "initialize agentInfo")
	local agent = fields(
		value.result.agentCapabilities,
		{ loadSession = true, mcpCapabilities = true, promptCapabilities = true },
		"initialize agentCapabilities"
	)
	if agent.loadSession ~= nil and type(agent.loadSession) ~= "boolean" then
		fail("initialize agentCapabilities.loadSession must be a boolean")
	end
	local mcp = agent.mcpCapabilities
		and fields(agent.mcpCapabilities, { http = true, sse = true }, "initialize mcpCapabilities")
	local prompt = agent.promptCapabilities
		and fields(
			agent.promptCapabilities,
			{ image = true, audio = true, embeddedContext = true },
			"initialize promptCapabilities"
		)
	for _, entry in ipairs({
		{ mcp, "http" },
		{ mcp, "sse" },
		{ prompt, "image" },
		{ prompt, "audio" },
		{ prompt, "embeddedContext" },
	}) do
		if entry[1] and entry[1][entry[2]] ~= nil and type(entry[1][entry[2]]) ~= "boolean" then
			fail("initialize capability " .. entry[2] .. " must be a boolean")
		end
	end
	return {
		agent = {
			name = string(info.name, "initialize agentInfo.name"),
			version = string(info.version, "initialize agentInfo.version"),
		},
		capabilities = {
			load_session = capability(agent, "loadSession"),
			mcp_http = capability(mcp, "http"),
			mcp_sse = capability(mcp, "sse"),
			images = capability(prompt, "image"),
			audio = capability(prompt, "audio"),
			embedded_context = capability(prompt, "embeddedContext"),
		},
	}
end

local function status(value)
	return vim.deepcopy({
		provider = "gemini",
		state = value.state,
		reason = value.reason,
		agent = value.agent,
		capabilities = value.capabilities,
	})
end

function M.command(executable)
	return { string(executable or "gemini", "executable"), "--acp" }
end

function M.new(opts)
	if type(opts) ~= "table" or type(opts.request) ~= "function" then
		fail("new requires a request callback")
	end
	for key in pairs(opts) do
		if key ~= "request" and key ~= "id" and key ~= "client_info" and key ~= "executable" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	local id = opts.id or "gator-handshake"
	if type(id) ~= "string" and (type(id) ~= "number" or id % 1 ~= 0) then
		fail("id must be a string or integer")
	end
	local client = opts.client_info or { name = "gator", version = "1" }
	fields(client, { name = true, title = true, version = true }, "client_info")
	string(client.name, "client_info.name")
	string(client.version, "client_info.version")
	if client.title ~= nil then
		string(client.title, "client_info.title")
	end
	return setmetatable({
		request = opts.request,
		id = id,
		client_info = vim.deepcopy(client),
		command = M.command(opts.executable),
		state = "ready",
	}, Handshake)
end

function M.unavailable(value)
	return setmetatable(
		{ state = "unavailable", reason = reason(value, "Gemini ACP transport is unavailable") },
		Handshake
	)
end

function M.is(value)
	return getmetatable(value) == Handshake
end

function Handshake:status()
	if not M.is(self) then
		fail("status requires a Gemini handshake")
	end
	return status(self)
end

function Handshake:cancel(value)
	if not M.is(self) then
		fail("cancel requires a Gemini handshake")
	end
	if self.state ~= "ready" then
		return false
	end
	self.state, self.reason = "cancelled", reason(value, "Gemini handshake cancelled")
	return true
end

function Handshake:connect()
	if not M.is(self) then
		fail("connect requires a Gemini handshake")
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
		self.reason = redact.text(ok and "Gemini initialize returned an invalid response" or tostring(value))
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
