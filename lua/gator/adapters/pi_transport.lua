local fixtures = require("gator.adapters.fixtures")
local redact = require("gator.policy.redact")
local M = { schema_version = 1 }
local Transport = {}

Transport.__index = Transport

local function fail(message)
	error("Gator Pi transport schema: " .. redact.text(tostring(message)), 3)
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

local function safe(value, name)
	if type(value) ~= "table" then
		return
	end
	if vim.islist(value) then
		for index, child in ipairs(value) do
			safe(child, name .. "[" .. index .. "]")
		end
		return
	end
	for key, child in pairs(value) do
		if type(key) ~= "string" then
			fail(name .. " keys must be strings")
		end
		local lower = key:lower()
		if
			lower:match("token")
			or lower:match("secret")
			or lower:match("credential")
			or lower:match("password")
			or lower:match("authorization")
			or lower:match("api[_-]?key")
			or lower:match("access[_-]?key")
		then
			fail(name .. " must not record credentials")
		end
		safe(child, name .. "." .. key)
	end
end

local function content(value, name)
	if type(value) ~= "table" or not vim.islist(value) then
		fail(name .. " must be an array")
	end
	for index, part in ipairs(value) do
		fields(part, { type = true, text = true }, name .. "[" .. index .. "]")
		if part.type ~= "text" then
			fail(name .. "[" .. index .. "] type must be text")
		end
		string(part.text, name .. "[" .. index .. "].text")
	end
end

local function message(value, name)
	fields(value, { role = true, content = true }, name)
	if value.role ~= "assistant" then
		fail(name .. " role must be assistant")
	end
	content(value.content, name .. ".content")
end

local function response(value, index)
	fields(
		value,
		{ id = true, type = true, command = true, success = true, data = true, error = true },
		"response " .. index
	)
	string(value.id, "response " .. index .. " id")
	local command = string(value.command, "response " .. index .. " command")
	if command ~= "get_state" and command ~= "get_commands" and command ~= "prompt" then
		fail("response " .. index .. " command is unsupported: " .. command)
	end
	if type(value.success) ~= "boolean" then
		fail("response " .. index .. " success must be a boolean")
	end
	if not value.success then
		if value.error ~= nil then
			string(value.error, "response " .. index .. " error")
		end
		return "failure", command
	end
	if value.error ~= nil then
		fail("successful response " .. index .. " cannot include error")
	end
	if command == "get_state" then
		fields(value.data, { sessionId = true, sessionFile = true, isStreaming = true }, "get_state data")
		string(value.data.sessionId, "get_state data.sessionId")
		string(value.data.sessionFile, "get_state data.sessionFile")
		if type(value.data.isStreaming) ~= "boolean" then
			fail("get_state data.isStreaming must be a boolean")
		end
	elseif command == "get_commands" then
		fields(value.data, { commands = true }, "get_commands data")
		if type(value.data.commands) ~= "table" or not vim.islist(value.data.commands) then
			fail("get_commands data.commands must be an array")
		end
		for command_index, command_data in ipairs(value.data.commands) do
			fields(
				command_data,
				{ name = true, description = true, source = true, location = true, path = true },
				"command " .. command_index
			)
			for _, key in ipairs({ "name", "description", "source", "location", "path" }) do
				string(command_data[key], "command " .. command_index .. "." .. key)
			end
		end
	elseif value.data ~= nil then
		fail("prompt response cannot include data")
	end
	return "response", command
end

local function event(value, index, prompted, ended)
	if value.type == "message_end" then
		fields(value, { type = true, message = true }, "event " .. index)
		if not prompted then
			fail("message_end must follow a successful prompt response")
		end
		message(value.message, "message_end message")
		return true
	end
	if value.type == "agent_end" then
		fields(value, { type = true, messages = true }, "event " .. index)
		if not ended then
			fail("agent_end must follow message_end")
		end
		if type(value.messages) ~= "table" or not vim.islist(value.messages) or #value.messages == 0 then
			fail("agent_end messages must be a non-empty array")
		end
		for message_index, item in ipairs(value.messages) do
			message(item, "agent_end messages[" .. message_index .. "]")
		end
		return ended
	end
	fail("event " .. index .. " type is unsupported: " .. tostring(value.type))
end

function M.new(opts)
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	for key in pairs(opts) do
		if key ~= "available" and key ~= "reason" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	if opts.available == false then
		return M.unavailable(opts.reason)
	end
	if opts.available ~= nil and type(opts.available) ~= "boolean" then
		fail("available must be a boolean")
	end
	if opts.reason ~= nil then
		fail("reason is only valid when transport is unavailable")
	end
	return setmetatable({ state = "ready" }, Transport)
end

function M.unavailable(reason)
	return setmetatable({ state = "unavailable", reason = redact.text(string(reason, "reason")) }, Transport)
end

function M.is(value)
	return getmetatable(value) == Transport
end

function Transport:status()
	if not M.is(self) then
		fail("status requires a transport schema")
	end
	return vim.deepcopy({ provider = "pi", schema_version = M.schema_version, state = self.state, reason = self.reason })
end

function Transport:cancel(reason)
	if not M.is(self) then
		fail("cancel requires a transport schema")
	end
	if self.state ~= "ready" then
		return false
	end
	self.state = "cancelled"
	self.reason = reason and redact.text(string(reason, "cancellation reason")) or nil
	return true
end

function Transport:replay(path, callback)
	if not M.is(self) then
		fail("replay requires a transport schema")
	end
	if self.state == "unavailable" then
		fail("transport schema is unavailable: " .. self.reason)
	end
	if self.state == "cancelled" then
		fail("transport schema is cancelled" .. (self.reason and ": " .. self.reason or ""))
	end
	if type(path) ~= "string" or path == "" then
		fail("replay path must be a non-empty string")
	end
	if callback ~= nil and type(callback) ~= "function" then
		fail("replay callback must be a function")
	end
	local prompted, ended = false, false
	local result = { records = 0, responses = 0, events = 0, failures = 0 }
	fixtures.replay_jsonl(path, function(record, index)
		safe(record, "record " .. index)
		local kind, command
		if record.type == "response" then
			kind, command = response(record, index)
			if command == "prompt" and kind == "response" then
				prompted = true
			end
			result.responses = result.responses + 1
			if kind == "failure" then
				result.failures = result.failures + 1
			end
		else
			ended = event(record, index, prompted, ended)
			kind = "event"
			result.events = result.events + 1
		end
		result.records = result.records + 1
		if callback then
			callback(vim.deepcopy(record), { index = index, kind = kind })
		end
	end)
	return result
end

return M
