local redact = require("gator.policy.redact")
local M = {}
local Handshake = {}

Handshake.__index = Handshake

local function fail(message)
	error("Gator Codex handshake: " .. redact.text(tostring(message)), 3)
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

local function reason(value, fallback)
	return type(value) == "string" and value ~= "" and redact.text(value) or fallback
end

local function absolute_path(value)
	return value:sub(1, 1) == "/" or value:match("^%a:[/\\]") ~= nil
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

local function client_info(value)
	fields(value, { name = true, title = true, version = true }, "client_info")
	string(value.name, "client_info.name")
	string(value.version, "client_info.version")
	if value.title ~= nil then
		string(value.title, "client_info.title")
	end
	return vim.deepcopy(value)
end

local function response(value, id)
	if value.error ~= nil then
		fields(value, { id = true, error = true }, "initialize response")
		if value.id ~= id then
			fail("initialize response does not match the handshake")
		end
		fields(value.error, { code = true, message = true, data = true }, "initialize response error")
		if type(value.error.code) ~= "number" or value.error.code % 1 ~= 0 then
			fail("initialize response error.code must be an integer")
		end
		return nil, reason(string(value.error.message, "initialize response error.message"), "Codex initialize failed")
	end
	fields(value, { id = true, result = true }, "initialize response")
	if value.id ~= id then
		fail("initialize response does not match the handshake")
	end
	fields(
		value.result,
		{ userAgent = true, codexHome = true, platformFamily = true, platformOs = true },
		"initialize result"
	)
	string(value.result.userAgent, "initialize result.userAgent")
	local home = string(value.result.codexHome, "initialize result.codexHome")
	if not absolute_path(home) then
		fail("initialize result.codexHome must be an absolute path")
	end
	return {
		platform_family = string(value.result.platformFamily, "initialize result.platformFamily"),
		platform_os = string(value.result.platformOs, "initialize result.platformOs"),
	}
end

local function status(value)
	return vim.deepcopy({
		provider = "codex",
		state = value.state,
		reason = value.reason,
		platform_family = value.platform_family,
		platform_os = value.platform_os,
	})
end

function M.new(opts)
	if type(opts) ~= "table" or type(opts.request) ~= "function" then
		fail("new requires a request callback")
	end
	for key in pairs(opts) do
		if key ~= "request" and key ~= "id" and key ~= "client_info" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	local info = opts.client_info or { name = "gator", version = "1" }
	return setmetatable({
		request = opts.request,
		id = request_id(opts.id or "gator-handshake", "id"),
		client_info = client_info(info),
		state = "ready",
	}, Handshake)
end

function M.unavailable(value)
	return setmetatable({ state = "unavailable", reason = reason(value, "Codex transport is unavailable") }, Handshake)
end

function M.is(value)
	return getmetatable(value) == Handshake
end

function Handshake:status()
	if not M.is(self) then
		fail("status requires a Codex handshake")
	end
	return status(self)
end

function Handshake:cancel(value)
	if not M.is(self) then
		fail("cancel requires a Codex handshake")
	end
	if self.state ~= "ready" then
		return false
	end
	self.state, self.reason = "cancelled", reason(value, "Codex handshake cancelled")
	return true
end

function Handshake:connect()
	if not M.is(self) then
		fail("connect requires a Codex handshake")
	end
	if self.state ~= "ready" then
		return status(self)
	end
	local ok, value = pcall(self.request, {
		id = self.id,
		method = "initialize",
		params = { clientInfo = vim.deepcopy(self.client_info) },
	})
	if not ok or type(value) ~= "table" then
		self.state = "failed"
		self.reason = redact.text(ok and "Codex initialize returned an invalid response" or tostring(value))
		return status(self)
	end
	local valid, result, detail = pcall(response, value, self.id)
	if not valid then
		self.state, self.reason = "failed", redact.text(tostring(result))
		return status(self)
	end
	if result == nil then
		self.state, self.reason = "failed", detail
		return status(self)
	end
	self.platform_family, self.platform_os = result.platform_family, result.platform_os
	return status(self)
end

return M
