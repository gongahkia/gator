local redact = require("gator.policy.redact")
local M = {}
local Control = {}

Control.__index = Control

local function fail(message)
	error("Gator Pi control: " .. redact.text(tostring(message)), 3)
end

local function reason(value, fallback)
	return type(value) == "string" and value ~= "" and redact.text(value) or fallback
end

local function status(value)
	return vim.deepcopy({ provider = "pi", state = value.state, reason = value.reason, session = value.session })
end

local function response(value, id, command)
	if type(value) ~= "table" or value.id ~= id or value.type ~= "response" or value.command ~= command then
		return nil, "Pi " .. command .. " returned an invalid response"
	end
	if value.success ~= true then
		return nil, reason(value.error, "Pi " .. command .. " failed")
	end
	return value
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
	local id = opts.id or "pi-control"
	if type(id) ~= "string" or not id:match("^[a-z][a-z0-9_-]*$") then
		fail("id must be a lowercase identifier")
	end
	return setmetatable({ request = opts.request, id = id, state = "ready" }, Control)
end

function M.unavailable(value)
	return setmetatable({ state = "unavailable", reason = reason(value, "Pi transport is unavailable") }, Control)
end

function M.is(value)
	return getmetatable(value) == Control
end

function Control:status()
	if not M.is(self) then
		fail("status requires Pi control")
	end
	return status(self)
end

function Control:cancel(reason_value)
	if not M.is(self) then
		fail("cancel requires Pi control")
	end
	if self.state == "unavailable" or self.state == "cancelled" then
		return status(self)
	end
	local ok, value = pcall(self.request, { id = self.id, type = "abort" })
	local valid, detail
	if ok then
		valid, detail = response(value, self.id, "abort")
	else
		detail = tostring(value)
	end
	if not valid then
		self.state, self.reason = "failed", redact.text(detail or "Pi abort failed")
		return status(self)
	end
	self.state, self.reason = "cancelled", reason(reason_value, "Pi run cancelled")
	return status(self)
end

function Control:recover()
	if not M.is(self) then
		fail("recover requires Pi control")
	end
	if self.state == "unavailable" then
		return status(self)
	end
	local ok, value = pcall(self.request, { id = self.id, type = "get_state" })
	local valid, detail
	if ok then
		valid, detail = response(value, self.id, "get_state")
	else
		detail = tostring(value)
	end
	if not valid or type(valid.data) ~= "table" or type(valid.data.isStreaming) ~= "boolean" then
		self.state, self.reason = "failed", redact.text(detail or "Pi get_state returned invalid recovery data")
		return status(self)
	end
	self.session = nil
	if valid.data.sessionFile ~= nil then
		if type(valid.data.sessionFile) ~= "string" or valid.data.sessionFile == "" then
			self.state, self.reason = "failed", "Pi get_state returned invalid session ownership"
			return status(self)
		end
		self.session = { provider = "pi", id = valid.data.sessionFile, owner = "provider" }
	end
	self.state, self.reason = "ready", nil
	return status(self)
end

return M
