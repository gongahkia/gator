local redact = require("gator.policy.redact")
local M = {}
local Handshake = {}

Handshake.__index = Handshake

local function fail(message)
	error("Gator Pi handshake: " .. redact.text(tostring(message)), 3)
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

local function reason(value, name)
	if type(value) ~= "string" or value == "" then
		return name
	end
	return redact.text(value)
end

local function state(value)
	if type(value) ~= "table" or vim.islist(value) then
		fail("get_state data must be an object")
	end
	if type(value.isStreaming) ~= "boolean" then
		fail("get_state data.isStreaming must be a boolean")
	end
	local result = { streaming = value.isStreaming == true }
	if value.sessionId ~= nil then
		string(value.sessionId, "get_state data.sessionId")
	end
	if value.sessionFile ~= nil then
		result.session =
			{ provider = "pi", id = string(value.sessionFile, "get_state data.sessionFile"), owner = "provider" }
	end
	return result
end

local function status(value)
	return {
		provider = "pi",
		state = value.state,
		reason = value.reason,
		streaming = value.streaming,
		session = value.session and vim.deepcopy(value.session) or nil,
	}
end

function M.new(opts)
	if type(opts) ~= "table" or type(opts.request) ~= "function" then
		fail("new requires a request callback")
	end
	for key in pairs(opts) do
		if key ~= "request" and key ~= "id" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	local id = opts.id or "gator-handshake"
	if type(id) ~= "string" or not id:match("^[a-z][a-z0-9_-]*$") then
		fail("id must be a lowercase identifier")
	end
	return setmetatable({ request = opts.request, id = id, state = "ready" }, Handshake)
end

function M.unavailable(reason_value)
	return setmetatable(
		{ state = "unavailable", reason = reason(reason_value, "Pi transport is unavailable") },
		Handshake
	)
end

function M.is(value)
	return getmetatable(value) == Handshake
end

function Handshake:status()
	if not M.is(self) then
		fail("status requires a Pi handshake")
	end
	return status(self)
end

function Handshake:cancel(reason_value)
	if not M.is(self) then
		fail("cancel requires a Pi handshake")
	end
	if self.state ~= "ready" then
		return false
	end
	self.state = "cancelled"
	self.reason = reason(reason_value, "Pi handshake cancelled")
	return true
end

function Handshake:connect()
	if not M.is(self) then
		fail("connect requires a Pi handshake")
	end
	if self.state == "unavailable" or self.state == "cancelled" then
		return status(self)
	end
	if self.state ~= "ready" then
		return status(self)
	end
	local ok, response = pcall(self.request, { type = "get_state", id = self.id })
	if not ok or type(response) ~= "table" then
		self.state = "failed"
		self.reason = redact.text(ok and "Pi get_state returned an invalid response" or tostring(response))
		return status(self)
	end
	local valid, result, detail = pcall(function()
		fields(
			response,
			{ id = true, type = true, command = true, success = true, data = true, error = true },
			"get_state response"
		)
		if response.id ~= self.id or response.type ~= "response" or response.command ~= "get_state" then
			fail("get_state response does not match the handshake")
		end
		if response.success ~= true then
			return nil, reason(response.error, "Pi get_state failed")
		end
		return state(response.data)
	end)
	if not valid then
		self.state = "failed"
		self.reason = redact.text(tostring(result))
		return status(self)
	end
	if result == nil then
		self.state = "failed"
		self.reason = detail
		return status(self)
	end
	self.state = "ready"
	self.streaming, self.session = result.streaming, result.session
	return status(self)
end

return M
