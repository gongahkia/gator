local fixtures = require("gator.adapters.fixtures")
local redact = require("gator.policy.redact")
local M = { schema_version = 2 }
local Transport = {}

Transport.__index = Transport

local plan_types = {
	free = true,
	go = true,
	plus = true,
	pro = true,
	prolite = true,
	team = true,
	self_serve_business_usage_based = true,
	business = true,
	enterprise_cbp_usage_based = true,
	enterprise = true,
	edu = true,
	unknown = true,
}

local function fail(message)
	error("Gator Codex transport schema: " .. redact.text(tostring(message)), 3)
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

local function string(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty string")
	end
	return value
end

local function request_id(value, name)
	if type(value) == "string" then
		return string(value, name)
	end
	if type(value) ~= "number" or value % 1 ~= 0 then
		fail(name .. " must be a string or integer")
	end
	return value
end

local function sensitive(key)
	key = key:lower()
	return key:match("token")
		or key:match("secret")
		or key:match("credential")
		or key:match("password")
		or key:match("authorization")
		or key:match("api[_-]?key")
		or key:match("access[_-]?key")
end

local function safe(value, name)
	if type(value) ~= "table" then
		return
	end
	for key, child in pairs(value) do
		if type(key) ~= "string" then
			fail(name .. " keys must be strings")
		end
		if
			sensitive(key)
			and not (
				(key == "refreshToken" and type(child) == "boolean")
				or (key == "credentialSource" and type(child) == "string")
			)
		then
			fail(name .. " must not record credentials")
		end
		safe(child, name .. "." .. key)
	end
end

local function client_info(value)
	fields(value, { name = true, title = true, version = true }, "initialize clientInfo")
	string(value.name, "initialize clientInfo.name")
	string(value.version, "initialize clientInfo.version")
	if value.title ~= nil and value.title ~= vim.NIL then
		string(value.title, "initialize clientInfo.title")
	end
end

local function initialize(value)
	fields(value, { clientInfo = true, capabilities = true }, "initialize params")
	client_info(value.clientInfo)
	if value.capabilities ~= nil and value.capabilities ~= vim.NIL then
		fields(value.capabilities, {
			experimentalApi = true,
			mcpServerOpenaiFormElicitation = true,
			optOutNotificationMethods = true,
			requestAttestation = true,
		}, "initialize capabilities")
		for _, key in ipairs({ "experimentalApi", "mcpServerOpenaiFormElicitation", "requestAttestation" }) do
			if value.capabilities[key] ~= nil and type(value.capabilities[key]) ~= "boolean" then
				fail("initialize capabilities." .. key .. " must be a boolean")
			end
		end
		if
			value.capabilities.optOutNotificationMethods ~= nil
			and value.capabilities.optOutNotificationMethods ~= vim.NIL
		then
			if
				type(value.capabilities.optOutNotificationMethods) ~= "table"
				or not vim.islist(value.capabilities.optOutNotificationMethods)
			then
				fail("initialize capabilities.optOutNotificationMethods must be an array")
			end
			for index, method in ipairs(value.capabilities.optOutNotificationMethods) do
				string(method, "initialize capabilities.optOutNotificationMethods[" .. index .. "]")
			end
		end
	end
end

local function account(value)
	if value == nil or value == vim.NIL then
		return
	end
	fields(value, { type = true, email = true, planType = true, credentialSource = true }, "account")
	local kind = string(value.type, "account.type")
	if kind == "apiKey" then
		return
	end
	if kind == "chatgpt" then
		if value.email == nil then
			fail("account.email is required for chatgpt")
		end
		if value.email ~= vim.NIL then
			string(value.email, "account.email")
		end
		if not plan_types[value.planType] then
			fail("account.planType must be a documented Codex plan type")
		end
		return
	end
	if kind == "amazonBedrock" then
		if value.credentialSource ~= nil and value.credentialSource ~= "awsManaged" then
			fail("account.credentialSource must be awsManaged")
		end
		return
	end
	fail("account.type is unsupported: " .. kind)
end

local function response(message, request, index)
	if message.error ~= nil then
		fields(message, { id = true, error = true }, "response " .. index)
		fields(message.error, { code = true, message = true, data = true }, "response " .. index .. " error")
		if type(message.error.code) ~= "number" or message.error.code % 1 ~= 0 then
			fail("response " .. index .. " error.code must be an integer")
		end
		string(message.error.message, "response " .. index .. " error.message")
		return "failure"
	end
	fields(message, { id = true, result = true }, "response " .. index)
	if request.method == "initialize" then
		fields(
			message.result,
			{ userAgent = true, codexHome = true, platformFamily = true, platformOs = true },
			"initialize result"
		)
		string(message.result.userAgent, "initialize result.userAgent")
		if message.result.codexHome ~= nil then
			local home = string(message.result.codexHome, "initialize result.codexHome")
			if home:sub(1, 1) ~= "/" then
				fail("initialize result.codexHome must be an absolute path")
			end
		end
		string(message.result.platformFamily, "initialize result.platformFamily")
		string(message.result.platformOs, "initialize result.platformOs")
	elseif request.method == "account/read" then
		fields(message.result, { account = true, requiresOpenaiAuth = true }, "account/read result")
		if type(message.result.requiresOpenaiAuth) ~= "boolean" then
			fail("account/read result.requiresOpenaiAuth must be a boolean")
		end
		account(message.result.account)
	end
	return "response"
end

local function request(message, index)
	local name = string(message.method, "request " .. index .. " method")
	fields(message, { id = true, method = true, params = true }, "request " .. index)
	request_id(message.id, "request " .. index .. " id")
	if name == "initialize" then
		initialize(message.params)
	elseif name == "account/read" then
		fields(message.params, { refreshToken = true }, "account/read params")
		if message.params.refreshToken ~= nil and type(message.params.refreshToken) ~= "boolean" then
			fail("account/read params.refreshToken must be a boolean")
		end
	else
		fail("request " .. index .. " method is unsupported: " .. name)
	end
	return name
end

local function notification(message, index)
	fields(message, { method = true, params = true }, "notification " .. index)
	local name = string(message.method, "notification " .. index .. " method")
	if name == "initialized" and message.params ~= nil then
		fields(message.params, {}, "initialized params")
	end
	return name
end

function M.new(opts)
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	for key in pairs(opts) do
		if key ~= "available" and key ~= "reason" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	if opts.available == false then
		return M.unavailable(opts.reason)
	end
	if opts.available ~= nil and type(opts.available) ~= "boolean" then
		fail("available must be a boolean")
	end
	if opts.reason ~= nil then
		fail("reason is only valid when transport is unavailable")
	end
	return setmetatable({ state = "ready" }, Transport)
end

function M.unavailable(reason)
	return setmetatable({ state = "unavailable", reason = redact.text(string(reason, "reason")) }, Transport)
end

function M.is(value)
	return getmetatable(value) == Transport
end

function Transport:status()
	if not M.is(self) then
		fail("status requires a transport schema")
	end
	return vim.deepcopy({
		provider = "codex",
		schema_version = M.schema_version,
		state = self.state,
		reason = self.reason,
	})
end

function Transport:cancel(reason)
	if not M.is(self) then
		fail("cancel requires a transport schema")
	end
	if self.state ~= "ready" then
		return false
	end
	self.state = "cancelled"
	self.reason = reason and redact.text(string(reason, "cancellation reason")) or nil
	return true
end

function Transport:replay(path, callback)
	if not M.is(self) then
		fail("replay requires a transport schema")
	end
	if self.state == "unavailable" then
		fail("transport schema is unavailable: " .. self.reason)
	end
	if self.state == "cancelled" then
		fail("transport schema is cancelled" .. (self.reason and ": " .. self.reason or ""))
	end
	if type(path) ~= "string" or path == "" then
		fail("replay path must be a non-empty string")
	end
	if callback ~= nil and type(callback) ~= "function" then
		fail("replay callback must be a function")
	end
	local pending = {}
	local result = { records = 0, requests = 0, notifications = 0, responses = 0, failures = 0 }
	fixtures.replay_jsonl(path, function(message, index)
		safe(message, "record " .. index)
		local kind
		if message.method ~= nil then
			if message.id ~= nil then
				local name = request(message, index)
				if pending[message.id] then
					fail("request " .. index .. " reuses a pending id")
				end
				pending[message.id] = { method = name }
				kind = "request"
				result.requests = result.requests + 1
			else
				notification(message, index)
				kind = "notification"
				result.notifications = result.notifications + 1
			end
		else
			if message.id == nil then
				fail("response " .. index .. " must include an id")
			end
			local id = request_id(message.id, "response " .. index .. " id")
			local pending_request = pending[id]
			if not pending_request then
				fail("response " .. index .. " does not match a request")
			end
			local state = response(message, pending_request, index)
			pending[id] = nil
			kind = state == "failure" and "failure" or "response"
			result.responses = result.responses + 1
			if kind == "failure" then
				result.failures = result.failures + 1
			end
		end
		result.records = result.records + 1
		if callback then
			callback(vim.deepcopy(message), { index = index, kind = kind })
		end
	end)
	for id in pairs(pending) do
		fail("fixture ended before response for request id " .. tostring(id))
	end
	return result
end

return M
