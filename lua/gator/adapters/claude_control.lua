local redact = require("gator.policy.redact")
local M = {}
local Control = {}

Control.__index = Control

local function fail(message)
	error("Gator Claude control: " .. redact.text(tostring(message)), 3)
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
	if value.provider ~= "claude" or value.owner ~= "provider" then
		fail("session must remain provider-owned by Claude")
	end
	return { provider = "claude", id = text(value.id, "session.id"), owner = "provider" }
end

local function init(value, value_session)
	if type(value) ~= "table" or vim.islist(value) then
		return nil, "Claude resume returned an invalid native event"
	end
	if value.type ~= "system" or value.subtype ~= "init" or value.session_id ~= value_session.id then
		return nil, "Claude resume changed provider session ownership"
	end
	return true
end

local function status(value)
	return vim.deepcopy({ provider = "claude", state = value.state, reason = value.reason, session = value.session })
end

function M.new(opts)
	if type(opts) ~= "table" or type(opts.interrupt) ~= "function" or type(opts.resume) ~= "function" then
		fail("new requires interrupt and resume callbacks")
	end
	for key in pairs(opts) do
		if key ~= "interrupt" and key ~= "resume" and key ~= "session" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	return setmetatable(
		{ interrupt = opts.interrupt, resume = opts.resume, session = session(opts.session), state = "ready" },
		Control
	)
end

function M.unavailable(value)
	return setmetatable({ state = "unavailable", reason = reason(value, "Claude transport is unavailable") }, Control)
end

function M.is(value)
	return getmetatable(value) == Control
end

function Control:status()
	if not M.is(self) then
		fail("status requires Claude control")
	end
	return status(self)
end

function Control:cancel(value)
	if not M.is(self) then
		fail("cancel requires Claude control")
	end
	if self.state == "unavailable" or self.state == "cancelled" then
		return status(self)
	end
	local ok, result = pcall(self.interrupt)
	if not ok or result == false then
		self.state, self.reason = "failed", reason(ok and nil or result, "Claude query interruption failed")
		return status(self)
	end
	self.state, self.reason = "cancelled", reason(value, "Claude query cancelled")
	return status(self)
end

function Control:recover()
	if not M.is(self) then
		fail("recover requires Claude control")
	end
	if self.state == "unavailable" then
		return status(self)
	end
	local ok, value = pcall(self.resume, self.session.id)
	if not ok then
		self.state, self.reason = "failed", redact.text(tostring(value))
		return status(self)
	end
	local valid, detail = init(value, self.session)
	if not valid then
		self.state, self.reason = "failed", reason(detail, "Claude session recovery failed")
		return status(self)
	end
	self.state, self.reason = "ready", nil
	return status(self)
end

return M
