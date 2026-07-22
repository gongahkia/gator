local session = require("gator.core.session")
local redact = require("gator.policy.redact")
local M = {
	states = { ready = true, pending = true, completed = true, unavailable = true, failed = true, cancelled = true },
}
local Request = {}

Request.__index = Request

local function fail(message)
	error("Gator handoff target: " .. redact.text(tostring(message)), 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function reason(value, fallback)
	if type(value) ~= "string" or value == "" then
		return fallback
	end
	return redact.text(value)
end

local function fields(value, allowed, name)
	for key in pairs(value) do
		if not allowed[key] then
			fail(name .. " contains unsupported field: " .. tostring(key))
		end
	end
end

local function launch(value)
	if type(value) ~= "table" or type(value.available) ~= "boolean" then
		fail("payload must declare availability")
	end
	if value.available == false then
		fields(
			value,
			{ available = true, provider = true, pack_id = true, task_id = true, reason = true },
			"unavailable payload"
		)
		if
			type(value.provider) ~= "string"
			or type(value.pack_id) ~= "string"
			or type(value.task_id) ~= "string"
			or type(value.reason) ~= "string"
			or value.reason == ""
		then
			fail("unavailable payload must identify provider, pack, and task")
		end
		local result = redact.value(vim.deepcopy(value))
		result.provider, result.pack_id, result.task_id = value.provider, value.pack_id, value.task_id
		return result
	end
	fields(value, {
		available = true,
		kind = true,
		provider = true,
		pack_id = true,
		task_id = true,
		target = true,
		prompt = true,
		context = true,
	}, "payload")
	if type(value.target) == "table" then
		fields(value.target, { session = true, transport = true, tools = true }, "payload target")
	end
	if type(value.target) == "table" and type(value.target.session) == "table" then
		fields(value.target.session, { mode = true, owner = true }, "payload target session")
	end
	if
		value.kind ~= "gator.handoff.launch"
		or type(value.provider) ~= "string"
		or type(value.pack_id) ~= "string"
		or type(value.task_id) ~= "string"
		or type(value.prompt) ~= "string"
		or value.prompt == ""
		or type(value.target) ~= "table"
		or type(value.target.session) ~= "table"
		or value.target.session.mode ~= "create"
		or value.target.session.owner ~= "provider"
		or value.target.session.id ~= nil
		or type(value.context) ~= "table"
		or type(value.context.entries) ~= "table"
		or not vim.islist(value.context.entries)
	then
		fail("payload must request a fresh provider-owned target session")
	end
	local result = redact.value(vim.deepcopy(value))
	result.provider, result.pack_id, result.task_id = value.provider, value.pack_id, value.task_id
	for index, entry in ipairs(value.context and value.context.entries or {}) do
		if result.context and result.context.entries[index] then
			result.context.entries[index].id = entry.id
		end
	end
	return result
end

local function unavailable(id, payload)
	return setmetatable({
		id = id,
		pack_id = payload.pack_id,
		task_id = payload.task_id,
		provider = payload.provider,
		state = "unavailable",
		reason = reason(payload.reason, "target handoff is unavailable"),
	}, Request)
end

function M.new(opts)
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	for key in pairs(opts) do
		if key ~= "id" and key ~= "payload" and key ~= "create" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	if type(opts.create) ~= "function" then
		fail("new requires a target session creation callback")
	end
	local id = identifier(opts.id, "id")
	local payload = launch(opts.payload)
	if not payload.available then
		return unavailable(id, payload)
	end
	return setmetatable({
		id = id,
		pack_id = payload.pack_id,
		task_id = payload.task_id,
		provider = payload.provider,
		state = "ready",
		payload = payload,
		create = opts.create,
	}, Request)
end

function M.is(value)
	return getmetatable(value) == Request
end

function Request:status()
	if not M.is(self) then
		fail("status requires a target handoff request")
	end
	local result = {
		id = self.id,
		pack_id = self.pack_id,
		task_id = self.task_id,
		provider = self.provider,
		state = self.state,
		reason = self.reason,
	}
	if self.value then
		result.session = session.reference(self.value)
	end
	return vim.deepcopy(result)
end

function Request:payload()
	if not M.is(self) then
		fail("payload requires a target handoff request")
	end
	if self.state ~= "ready" and self.state ~= "pending" then
		return nil
	end
	return vim.deepcopy(self.payload)
end

function Request:complete(native, detail)
	if not M.is(self) then
		fail("complete requires a target handoff request")
	end
	if self.state ~= "pending" then
		return false
	end
	if detail ~= nil then
		self.state = "failed"
		self.reason = reason(detail, "target provider failed to create a handoff session")
		return false
	end
	local ok, value = pcall(session.new, {
		task_id = self.task_id,
		provider = native and native.provider,
		id = native and native.id,
		owner = native and native.owner,
	})
	if not ok or value.provider ~= self.provider then
		self.state = "failed"
		self.reason = "target provider returned an invalid handoff session"
		return false
	end
	self.value = value
	self.state = "completed"
	return true
end

function Request:dispatch()
	if not M.is(self) then
		fail("dispatch requires a target handoff request")
	end
	if self.state ~= "ready" then
		return false
	end
	self.state = "pending"
	local ok, native, detail = pcall(self.create, vim.deepcopy(self.payload), function(value, failure)
		self:complete(value, failure)
	end)
	if not ok then
		if self.state == "pending" then
			self.state = "failed"
			self.reason = reason(native, "target provider failed to create a handoff session")
		end
		return false
	end
	if native == false and self.state == "pending" then
		self.state = "failed"
		self.reason = reason(detail, "target provider rejected the handoff session")
		return false
	end
	if type(native) == "table" and self.state == "pending" then
		return self:complete(native, detail)
	end
	if native ~= nil and native ~= true then
		self.state = "failed"
		self.reason = "target provider returned an unsupported handoff session result"
		return false
	end
	return self.state ~= "failed"
end

function Request:session()
	if not M.is(self) then
		fail("session requires a target handoff request")
	end
	if self.state ~= "completed" then
		return nil
	end
	return session.new({
		task_id = self.value.task_id,
		provider = self.value.provider,
		id = self.value.id,
		owner = self.value.owner,
	})
end

function Request:cancel(detail)
	if not M.is(self) then
		fail("cancel requires a target handoff request")
	end
	if self.state ~= "ready" and self.state ~= "pending" then
		return false
	end
	self.state = "cancelled"
	self.reason = reason(detail, "target handoff session creation was cancelled")
	return true
end

return M
