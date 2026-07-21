local redact = require("gator.policy.redact")
local M = {}
local Control = {}

Control.__index = Control

local function fail(message)
	error("Gator Gemini control: " .. redact.text(tostring(message)), 3)
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

local function text(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be non-empty text")
	end
	return value
end

local function reason(value, fallback)
	return type(value) == "string" and value ~= "" and redact.text(value) or fallback
end

local function session(value)
	fields(value, { provider = true, id = true, owner = true }, "session")
	if value.provider ~= "gemini" or value.owner ~= "provider" then
		fail("session must remain provider-owned by Gemini")
	end
	return { provider = "gemini", id = text(value.id, "session.id"), owner = "provider" }
end

local function cwd(value)
	value = text(value, "cwd")
	value = vim.uv.fs_realpath(value)
	if not value or vim.fn.isdirectory(value) ~= 1 then
		fail("cwd must resolve to a directory")
	end
	return value
end

local function response(value, id)
	if type(value) ~= "table" or value.id ~= id then
		return nil, "Gemini session/load returned an invalid response"
	end
	if value.error ~= nil then
		if type(value.error) ~= "table" then
			return nil, "Gemini session/load returned an invalid error"
		end
		return nil, reason(value.error.message, "Gemini session/load failed")
	end
	if type(value.result) ~= "table" or vim.islist(value.result) then
		return nil, "Gemini session/load returned an invalid result"
	end
	for key in pairs(value.result) do
		if key ~= "modes" and key ~= "configOptions" and key ~= "_meta" then
			return nil, "Gemini session/load returned unsupported recovery data"
		end
	end
	return value.result
end

local function status(value)
	return vim.deepcopy({ provider = "gemini", state = value.state, reason = value.reason, session = value.session })
end

function M.new(opts)
	if type(opts) ~= "table" or type(opts.notify) ~= "function" or type(opts.request) ~= "function" then
		fail("new requires notify and request callbacks")
	end
	for key in pairs(opts) do
		if key ~= "notify" and key ~= "request" and key ~= "session" and key ~= "cwd" and key ~= "id" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	local id = opts.id or "gemini-control"
	if type(id) ~= "string" or not id:match("^[a-z][a-z0-9_-]*$") then
		fail("id must be a lowercase identifier")
	end
	return setmetatable({
		notify = opts.notify,
		request = opts.request,
		session = session(opts.session),
		cwd = cwd(opts.cwd),
		id = id,
		sequence = 0,
		state = "ready",
	}, Control)
end

function M.unavailable(value)
	return setmetatable(
		{ state = "unavailable", reason = reason(value, "Gemini ACP transport is unavailable") },
		Control
	)
end

function M.is(value)
	return getmetatable(value) == Control
end

function Control:status()
	if not M.is(self) then
		fail("status requires Gemini control")
	end
	return status(self)
end

function Control:cancel(value)
	if not M.is(self) then
		fail("cancel requires Gemini control")
	end
	if self.state == "unavailable" or self.state == "cancelled" then
		return status(self)
	end
	local ok, result = pcall(self.notify, { method = "session/cancel", params = { sessionId = self.session.id } })
	if not ok or result == false then
		self.state, self.reason = "failed", reason(ok and nil or result, "Gemini session cancellation failed")
		return status(self)
	end
	self.state, self.reason = "cancelled", reason(value, "Gemini session cancelled")
	return status(self)
end

function Control:recover()
	if not M.is(self) then
		fail("recover requires Gemini control")
	end
	if self.state == "unavailable" then
		return status(self)
	end
	self.sequence = self.sequence + 1
	local id = self.id .. "-" .. self.sequence
	local ok, value = pcall(self.request, {
		id = id,
		method = "session/load",
		params = { sessionId = self.session.id, cwd = self.cwd, mcpServers = {} },
	})
	local result, detail
	if ok then
		result, detail = response(value, id)
	else
		detail = tostring(value)
	end
	if not result then
		self.state, self.reason = "failed", redact.text(detail or "Gemini session recovery failed")
		return status(self)
	end
	self.state, self.reason = "ready", nil
	return status(self)
end

return M
