local approval = require("gator.core.approval_event")
local redact = require("gator.policy.redact")
local M = {}
local Bridge = {}

Bridge.__index = Bridge

local function fail(message)
	error("Gator Codex permission bridge: " .. redact.text(tostring(message)), 3)
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

local function request_id(value)
	if type(value) == "string" then
		return text(value, "request id")
	end
	if type(value) ~= "number" or value % 1 ~= 0 then
		fail("request id must be a string or integer")
	end
	return value
end

local function integer(value, name)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail(name .. " must be a non-negative integer")
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

local function thread(value, request_context, name)
	value = text(value, name .. ".threadId")
	if request_context.session_id ~= nil and request_context.session_id ~= value then
		fail(name .. " belongs to a different provider-owned thread")
	end
	return value
end

local methods = {
	["item/commandExecution/requestApproval"] = { kind = "command", action = "execute command" },
	["item/fileChange/requestApproval"] = { kind = "file_change", action = "modify files" },
	["item/permissions/requestApproval"] = { kind = "permissions", action = "grant additional permissions" },
}

local function request_key(value)
	return type(value) .. ":" .. tostring(value)
end

local function parse(raw, request_context)
	fields(raw, { id = true, method = true, params = true }, "approval request")
	local native_id = request_id(raw.id)
	local method = text(raw.method, "approval request.method")
	local descriptor = methods[method]
	if not descriptor then
		return nil
	end
	local params = raw.params
	if descriptor.kind == "command" then
		fields(params, {
			turnId = true,
			approvalId = true,
			threadId = true,
			command = true,
			commandActions = true,
			cwd = true,
			environmentId = true,
			itemId = true,
			networkApprovalContext = true,
			proposedExecpolicyAmendment = true,
			proposedNetworkPolicyAmendments = true,
			reason = true,
			startedAtMs = true,
		}, method .. " params")
	elseif descriptor.kind == "file_change" then
		fields(
			params,
			{ grantRoot = true, itemId = true, reason = true, startedAtMs = true, threadId = true, turnId = true },
			method .. " params"
		)
	else
		fields(params, {
			cwd = true,
			environmentId = true,
			itemId = true,
			permissions = true,
			reason = true,
			startedAtMs = true,
			threadId = true,
			turnId = true,
		}, method .. " params")
		if type(params.permissions) ~= "table" or vim.islist(params.permissions) then
			fail(method .. " params.permissions must be an object")
		end
	end
	thread(params.threadId, request_context, method .. " params")
	text(params.turnId, method .. " params.turnId")
	text(params.itemId, method .. " params.itemId")
	integer(params.startedAtMs, method .. " params.startedAtMs")
	if params.reason ~= nil and type(params.reason) ~= "string" then
		fail(method .. " params.reason must be a string")
	end
	return {
		native_id = native_id,
		key = request_key(native_id),
		kind = descriptor.kind,
		action = descriptor.action,
		params = vim.deepcopy(params),
		details = { kind = descriptor.kind, item_id = params.itemId, turn_id = params.turnId, reason = params.reason },
	}
end

local function status(value)
	local pending = 0
	for _ in pairs(value.pending) do
		pending = pending + 1
	end
	return vim.deepcopy({ provider = "codex", state = value.state, reason = value.reason, pending = pending })
end

local function event(value, method, request_context, attrs)
	local sequence = value.sequence
	local ok, id = pcall(value.event_id, { provider = "codex", run_id = request_context.run_id, sequence = sequence })
	if not ok then
		fail("event_id callback failed: " .. tostring(id))
	end
	local at_ok, at = pcall(value.now)
	if not at_ok then
		fail("now callback failed: " .. tostring(at))
	end
	integer(at, "now callback result")
	attrs.id = id
	attrs.run_id = request_context.run_id
	attrs.provider = { name = "codex", session_id = request_context.session_id }
	attrs.sequence = sequence
	attrs.at = at
	local result = method(attrs)
	value.sequence = sequence + 1
	return result
end

local function response(value, decision)
	if value.kind == "permissions" then
		if decision == "approved" then
			return { permissions = vim.deepcopy(value.params.permissions), scope = "turn" }
		end
		return { permissions = {}, scope = "turn" }
	end
	local choices = {
		approved = "accept",
		denied = "decline",
		cancelled = "cancel",
	}
	return { decision = choices[decision] }
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
			return "codex-permission-" .. request_context.sequence
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
		reason = reason(value, "Codex permission transport is unavailable"),
		pending = {},
		native_pending = {},
	}, Bridge)
end

function M.is(value)
	return getmetatable(value) == Bridge
end

function Bridge:status()
	if not M.is(self) then
		fail("status requires a Codex permission bridge")
	end
	return status(self)
end

function Bridge:receive(raw, value)
	if not M.is(self) then
		fail("receive requires a Codex permission bridge")
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
		fail("native approval request is already pending")
	end
	self.request_sequence = self.request_sequence + 1
	request.id = "codex-approval-" .. self.request_sequence
	request.context = request_context
	local emitted = event(self, approval.request, request_context, {
		request_id = request.id,
		action = request.action,
		details = request.details,
	})
	self.pending[request.id] = request
	self.native_pending[request.key] = request.id
	return emitted
end

function Bridge:decide(id, decision)
	if not M.is(self) then
		fail("decide requires a Codex permission bridge")
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
	local ok, result = pcall(self.respond, { id = request.native_id, result = response(request, decision) })
	if not ok or result == false then
		self.state = "failed"
		self.reason = "Codex permission response failed"
		return status(self)
	end
	self.pending[id] = nil
	self.native_pending[request.key] = nil
	return decision_event
end

function Bridge:cancel(value)
	if not M.is(self) then
		fail("cancel requires a Codex permission bridge")
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
	self.state, self.reason = "cancelled", reason(value, "Codex permission bridge cancelled")
	return true
end

return M
