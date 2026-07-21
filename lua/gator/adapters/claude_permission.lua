local approval = require("gator.core.approval_event")
local redact = require("gator.policy.redact")
local M = {}
local Bridge = {}

Bridge.__index = Bridge

local function fail(message)
	error("Gator Claude permission bridge: " .. redact.text(tostring(message)), 3)
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

local function tool(value)
	value = text(value, "tool_name")
	if not value:match("^[A-Za-z][A-Za-z0-9_]*$") then
		fail("tool_name must be a native tool identifier")
	end
	return value
end

local function reason(value, fallback)
	return type(value) == "string" and value ~= "" and redact.text(value) or fallback
end

local function context(value)
	fields(value, { run_id = true, session_id = true }, "context")
	local result = { run_id = identifier(value.run_id, "context.run_id") }
	if value.session_id ~= nil then
		result.session_id = text(value.session_id, "context.session_id")
	end
	return result
end

local actions = {
	Bash = "execute command",
	Edit = "modify files",
	Read = "read files",
	Write = "modify files",
	AskUserQuestion = "answer question",
}

local function parse(raw)
	fields(raw, { input = true, tool_name = true }, "permission request")
	if type(raw.input) ~= "table" or vim.islist(raw.input) then
		fail("permission request.input must be an object")
	end
	local name = tool(raw.tool_name)
	return { tool = name, input = vim.deepcopy(raw.input), action = actions[name] or "use " .. name }
end

local function status(value)
	local pending = 0
	for _ in pairs(value.pending) do
		pending = pending + 1
	end
	return vim.deepcopy({ provider = "claude", state = value.state, reason = value.reason, pending = pending })
end

local function event(value, method, request_context, attrs)
	local sequence = value.sequence
	local ok, id = pcall(value.event_id, { provider = "claude", run_id = request_context.run_id, sequence = sequence })
	if not ok then
		fail("event_id callback failed: " .. tostring(id))
	end
	local at_ok, at = pcall(value.now)
	if not at_ok then
		fail("now callback failed: " .. tostring(at))
	end
	if type(at) ~= "number" or at < 0 or at % 1 ~= 0 then
		fail("now callback must return a non-negative integer")
	end
	attrs.id = id
	attrs.run_id = request_context.run_id
	attrs.provider = { name = "claude", session_id = request_context.session_id }
	attrs.sequence = sequence
	attrs.at = at
	local result = method(attrs)
	value.sequence = sequence + 1
	return result
end

local function response(request, decision)
	if decision == "approved" then
		return { behavior = "allow", updatedInput = vim.deepcopy(request.input) }
	end
	return {
		behavior = "deny",
		message = decision == "cancelled" and "Gator cancelled Claude tool request"
			or "Gator denied Claude tool request",
	}
end

function M.new(opts)
	if type(opts) ~= "table" or type(opts.respond) ~= "function" then
		fail("new requires a response callback")
	end
	for key in pairs(opts) do
		if key ~= "respond" and key ~= "event_id" and key ~= "now" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	if opts.event_id ~= nil and type(opts.event_id) ~= "function" then
		fail("event_id must be a function")
	end
	if opts.now ~= nil and type(opts.now) ~= "function" then
		fail("now must be a function")
	end
	return setmetatable({
		respond = opts.respond,
		event_id = opts.event_id or function(request_context)
			return "claude-permission-" .. request_context.sequence
		end,
		now = opts.now or os.time,
		state = "ready",
		pending = {},
		sequence = 0,
		request_sequence = 0,
	}, Bridge)
end

function M.unavailable(value)
	return setmetatable(
		{ state = "unavailable", reason = reason(value, "Claude permission transport is unavailable"), pending = {} },
		Bridge
	)
end

function M.is(value)
	return getmetatable(value) == Bridge
end

function Bridge:status()
	if not M.is(self) then
		fail("status requires a Claude permission bridge")
	end
	return status(self)
end

function Bridge:receive(raw, value)
	if not M.is(self) then
		fail("receive requires a Claude permission bridge")
	end
	if self.state == "unavailable" then
		fail("permission bridge is unavailable: " .. self.reason)
	end
	if self.state == "cancelled" then
		fail("permission bridge is cancelled" .. (self.reason and ": " .. self.reason or ""))
	end
	if self.state == "failed" then
		fail("permission bridge failed: " .. self.reason)
	end
	local request_context = context(value)
	local request = parse(raw)
	self.request_sequence = self.request_sequence + 1
	request.id = "claude-approval-" .. self.request_sequence
	request.context = request_context
	local emitted = event(self, approval.request, request_context, {
		request_id = request.id,
		action = request.action,
		details = { kind = "tool", tool = request.tool },
	})
	self.pending[request.id] = request
	return emitted
end

function Bridge:decide(id, decision)
	if not M.is(self) then
		fail("decide requires a Claude permission bridge")
	end
	if self.state ~= "ready" then
		return status(self)
	end
	id = text(id, "approval id")
	if decision ~= "approved" and decision ~= "denied" and decision ~= "cancelled" then
		fail("approval decision is unavailable")
	end
	local request = self.pending[id]
	if not request then
		fail("approval request is unavailable")
	end
	local decision_event = event(self, approval.decision, request.context, { request_id = id, decision = decision })
	local ok, result = pcall(self.respond, response(request, decision))
	if not ok or result == false then
		self.state, self.reason = "failed", "Claude permission response failed"
		return status(self)
	end
	self.pending[id] = nil
	return decision_event
end

function Bridge:cancel(value)
	if not M.is(self) then
		fail("cancel requires a Claude permission bridge")
	end
	if self.state ~= "ready" then
		return false
	end
	local ids = {}
	for id in pairs(self.pending) do
		table.insert(ids, id)
	end
	table.sort(ids)
	for _, id in ipairs(ids) do
		self:decide(id, "cancelled")
		if self.state == "failed" then
			return false
		end
	end
	self.state, self.reason = "cancelled", reason(value, "Claude permission bridge cancelled")
	return true
end

return M
