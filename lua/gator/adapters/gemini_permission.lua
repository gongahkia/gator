local approval = require("gator.core.approval_event")
local redact = require("gator.policy.redact")
local M = {}
local Bridge = {}

Bridge.__index = Bridge

local function fail(message)
	error("Gator Gemini permission bridge: " .. redact.text(tostring(message)), 3)
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

local function native_id(value)
	if type(value) == "string" then
		return text(value, "request id")
	end
	if type(value) ~= "number" or value % 1 ~= 0 then
		fail("request id must be a string or integer")
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

local function request_key(value)
	return type(value) .. ":" .. tostring(value)
end

local function option(value, index)
	fields(value, { optionId = true, name = true, kind = true, _meta = true }, "permission option " .. index)
	local kinds = { allow_once = true, allow_always = true, reject_once = true, reject_always = true }
	local kind = text(value.kind, "permission option " .. index .. ".kind")
	if not kinds[kind] then
		fail("permission option " .. index .. ".kind is unsupported")
	end
	return { id = text(value.optionId, "permission option " .. index .. ".optionId"), kind = kind }
end

local function parse(raw, request_context)
	fields(raw, { id = true, method = true, params = true }, "permission request")
	if text(raw.method, "permission request.method") ~= "session/request_permission" then
		return nil
	end
	local params =
		fields(raw.params, { sessionId = true, toolCall = true, options = true, _meta = true }, "permission params")
	local session_id = text(params.sessionId, "permission params.sessionId")
	if request_context.session_id ~= nil and request_context.session_id ~= session_id then
		fail("permission request belongs to a different provider-owned session")
	end
	local tool = fields(params.toolCall, {
		toolCallId = true,
		title = true,
		kind = true,
		status = true,
		content = true,
		locations = true,
		rawInput = true,
		rawOutput = true,
		_meta = true,
	}, "permission params.toolCall")
	local tool_call_id = text(tool.toolCallId, "permission params.toolCall.toolCallId")
	local kind = tool.kind == nil and "other" or text(tool.kind, "permission params.toolCall.kind")
	if type(params.options) ~= "table" or not vim.islist(params.options) or #params.options == 0 then
		fail("permission params.options must be a non-empty array")
	end
	local options = {}
	for index, value in ipairs(params.options) do
		options[index] = option(value, index)
	end
	local id = native_id(raw.id)
	return {
		native_id = id,
		key = request_key(id),
		tool_call_id = tool_call_id,
		kind = kind,
		options = options,
		context = request_context,
	}
end

local function status(value)
	local pending = 0
	for _ in pairs(value.pending) do
		pending = pending + 1
	end
	return vim.deepcopy({ provider = "gemini", state = value.state, reason = value.reason, pending = pending })
end

local function event(value, method, request_context, attrs)
	local sequence = value.sequence
	local ok, id = pcall(value.event_id, { provider = "gemini", run_id = request_context.run_id, sequence = sequence })
	if not ok then
		fail("event_id callback failed: " .. tostring(id))
	end
	local at_ok, at = pcall(value.now)
	if not at_ok or type(at) ~= "number" or at < 0 or at % 1 ~= 0 then
		fail("now callback must return a non-negative integer")
	end
	attrs.id = id
	attrs.run_id = request_context.run_id
	attrs.provider = { name = "gemini", session_id = request_context.session_id }
	attrs.sequence = sequence
	attrs.at = at
	local result = method(attrs)
	value.sequence = sequence + 1
	return result
end

local function response(request, decision)
	if decision == "cancelled" then
		return { id = request.native_id, result = { outcome = { outcome = "cancelled" } } }
	end
	local desired = decision == "approved" and { allow_once = true, allow_always = true }
		or { reject_once = true, reject_always = true }
	for _, option_value in ipairs(request.options) do
		if desired[option_value.kind] then
			return {
				id = request.native_id,
				result = { outcome = { outcome = "selected", optionId = option_value.id } },
			}
		end
	end
	return nil
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
			return "gemini-permission-" .. request_context.sequence
		end,
		now = opts.now or os.time,
		state = "ready",
		pending = {},
		native_pending = {},
		sequence = 0,
		request_sequence = 0,
	}, Bridge)
end

function M.unavailable(value)
	return setmetatable({
		state = "unavailable",
		reason = reason(value, "Gemini permission transport is unavailable"),
		pending = {},
		native_pending = {},
	}, Bridge)
end

function M.is(value)
	return getmetatable(value) == Bridge
end

function Bridge:status()
	if not M.is(self) then
		fail("status requires a Gemini permission bridge")
	end
	return status(self)
end

function Bridge:receive(raw, value)
	if not M.is(self) then
		fail("receive requires a Gemini permission bridge")
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
	local request = parse(raw, request_context)
	if request == nil then
		return {}
	end
	if self.native_pending[request.key] then
		fail("native permission request is already pending")
	end
	self.request_sequence = self.request_sequence + 1
	request.id = "gemini-approval-" .. self.request_sequence
	local emitted = event(self, approval.request, request_context, {
		request_id = request.id,
		action = "approve " .. request.kind .. " tool operation",
		details = { kind = request.kind, tool_call_id = request.tool_call_id },
	})
	self.pending[request.id] = request
	self.native_pending[request.key] = request.id
	return emitted
end

function Bridge:decide(id, decision)
	if not M.is(self) then
		fail("decide requires a Gemini permission bridge")
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
	local native = response(request, decision)
	local incompatible = native == nil
	if incompatible then
		native = response(request, "cancelled")
	end
	local ok, result = pcall(self.respond, native)
	if not ok or result == false then
		self.state, self.reason = "failed", "Gemini permission response failed"
		return status(self)
	end
	self.pending[id] = nil
	self.native_pending[request.key] = nil
	if incompatible then
		self.state, self.reason = "failed", "Gemini permission request cannot represent " .. decision
		return status(self)
	end
	return decision_event
end

function Bridge:cancel(value)
	if not M.is(self) then
		fail("cancel requires a Gemini permission bridge")
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
	self.state, self.reason = "cancelled", reason(value, "Gemini permission bridge cancelled")
	return true
end

return M
