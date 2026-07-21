local redact = require("gator.policy.redact")
local M = {}
local Control = {}

Control.__index = Control

local function fail(message)
	error("Gator Codex control: " .. redact.text(tostring(message)), 3)
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

local function identifier(value, name)
	value = text(value, name)
	if not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function reason(value, fallback)
	return type(value) == "string" and value ~= "" and redact.text(value) or fallback
end

local function session(value)
	fields(value, { provider = true, id = true, owner = true }, "session")
	if value.provider ~= "codex" or value.owner ~= "provider" then
		fail("session must remain provider-owned by Codex")
	end
	return { provider = "codex", id = text(value.id, "session.id"), owner = "provider" }
end

local function status(value)
	return vim.deepcopy({ provider = "codex", state = value.state, reason = value.reason, session = value.session })
end

local function response(value, id, method)
	if type(value) ~= "table" or value.id ~= id then
		return nil, "Codex " .. method .. " returned an invalid response"
	end
	if value.error ~= nil then
		if type(value.error) ~= "table" then
			return nil, "Codex " .. method .. " returned an invalid error"
		end
		return nil, reason(value.error.message, "Codex " .. method .. " failed")
	end
	if type(value.result) ~= "table" then
		return nil, "Codex " .. method .. " returned an invalid result"
	end
	return value.result
end

function M.new(opts)
	if type(opts) ~= "table" or type(opts.request) ~= "function" then
		fail("new requires a request callback")
	end
	for key in pairs(opts) do
		if key ~= "request" and key ~= "id" and key ~= "session" and key ~= "turn_id" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	return setmetatable({
		request = opts.request,
		id = identifier(opts.id or "codex-control", "id"),
		session = session(opts.session),
		turn_id = opts.turn_id and text(opts.turn_id, "turn_id") or nil,
		state = "ready",
		sequence = 0,
	}, Control)
end

function M.unavailable(value)
	return setmetatable({ state = "unavailable", reason = reason(value, "Codex transport is unavailable") }, Control)
end

function M.is(value)
	return getmetatable(value) == Control
end

function Control:status()
	if not M.is(self) then
		fail("status requires Codex control")
	end
	return status(self)
end

function Control:send(method, params)
	self.sequence = self.sequence + 1
	local id = self.id .. "-" .. self.sequence
	local ok, value = pcall(self.request, { id = id, method = method, params = vim.deepcopy(params) })
	if not ok then
		return nil, redact.text(tostring(value))
	end
	return response(value, id, method)
end

function Control:cancel(value)
	if not M.is(self) then
		fail("cancel requires Codex control")
	end
	if self.state == "unavailable" or self.state == "cancelled" then
		return status(self)
	end
	if not self.turn_id then
		self.state, self.reason = "failed", "Codex turn is unavailable for cancellation"
		return status(self)
	end
	local result, detail = self:send("turn/interrupt", { threadId = self.session.id, turnId = self.turn_id })
	if not result then
		self.state, self.reason = "failed", reason(detail, "Codex turn interruption failed")
		return status(self)
	end
	self.state, self.reason = "cancelled", reason(value, "Codex turn cancelled")
	return status(self)
end

function Control:recover()
	if not M.is(self) then
		fail("recover requires Codex control")
	end
	if self.state == "unavailable" then
		return status(self)
	end
	local result, detail = self:send("thread/resume", { threadId = self.session.id })
	if not result or type(result.thread) ~= "table" or vim.islist(result.thread) then
		self.state, self.reason = "failed", reason(detail, "Codex thread resume returned invalid recovery data")
		return status(self)
	end
	if result.thread.id ~= self.session.id then
		self.state, self.reason = "failed", "Codex thread resume changed provider session ownership"
		return status(self)
	end
	self.state, self.reason = "ready", nil
	return status(self)
end

return M
