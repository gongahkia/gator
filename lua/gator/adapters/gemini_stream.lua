local decoder = require("gator.adapters.decoder")
local redact = require("gator.policy.redact")
local M = {}
local Stream = {}

Stream.__index = Stream

local function fail(message)
	error("Gator Gemini stream: " .. redact.text(tostring(message)), 3)
end

local function text(value, name)
	if type(value) ~= "string" then
		fail(name .. " must be a string")
	end
	return value
end

local function object(value, name)
	if type(value) ~= "table" or vim.islist(value) then
		fail(name .. " must be an object")
	end
	return value
end

local function session(value, context, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. ".session_id must be non-empty text")
	end
	if context.session_id ~= nil and context.session_id ~= value then
		fail(name .. " belongs to a different provider-owned session")
	end
	return value
end

local function sensitive(key)
	key = key:lower()
	return key:match("token")
		or key:match("secret")
		or key:match("credential")
		or key:match("password")
		or key:match("authorization")
		or key:match("api[_-]?key")
		or key:match("access[_-]?key")
end

local function safe(value, name)
	if type(value) == "string" then
		return redact.text(value)
	end
	if type(value) == "number" or type(value) == "boolean" then
		return value
	end
	if type(value) ~= "table" then
		fail(name .. " must be JSON-compatible")
	end
	local result = {}
	if vim.islist(value) then
		for index, item in ipairs(value) do
			result[index] = safe(item, name .. "[" .. index .. "]")
		end
		return result
	end
	for key, item in pairs(value) do
		if type(key) ~= "string" then
			fail(name .. " keys must be strings")
		end
		if not sensitive(key) then
			result[key] = safe(item, name .. "." .. key)
		end
	end
	return result
end

local function message(value)
	object(value, "message event")
	local role = text(value.role, "message event.role")
	local content = text(value.content, "message event.content")
	if value.delta ~= nil and type(value.delta) ~= "boolean" then
		fail("message event.delta must be a boolean")
	end
	if role ~= "assistant" then
		return nil
	end
	return { type = value.delta == true and "message.delta" or "message.completed", payload = { text = content } }
end

local function tool_use(value)
	object(value, "tool-use event")
	local parameters = value.parameters or {}
	object(parameters, "tool-use event.parameters")
	return {
		type = "tool.call",
		payload = {
			call_id = text(value.tool_id, "tool-use event.tool_id"),
			name = text(value.tool_name, "tool-use event.tool_name"),
			input = safe(parameters, "tool-use event.parameters"),
		},
	}
end

local function tool_result(value)
	object(value, "tool-result event")
	local states = { success = "completed", error = "failed", cancelled = "cancelled", canceled = "cancelled" }
	local state = states[text(value.status, "tool-result event.status")]
	if not state then
		fail("tool-result event.status is unsupported")
	end
	return {
		type = "tool.result",
		payload = {
			call_id = text(value.tool_id, "tool-result event.tool_id"),
			state = state,
			output = value.output == nil and {} or safe(value.output, "tool-result event.output"),
		},
	}
end

local function result(value)
	object(value, "result event")
	local status = text(value.status, "result event.status")
	if status == "success" then
		return { type = "run.completed", payload = { status = "completed" } }
	end
	if status ~= "error" and status ~= "failed" and status ~= "cancelled" and status ~= "canceled" then
		fail("result event.status is unsupported")
	end
	local message = type(value.error) == "string" and redact.text(value.error) or "Gemini stream failed"
	return {
		type = "run.error",
		payload = { kind = "provider", message = message, retryable = false },
	}
end

local function decode(raw, context)
	object(raw, "stream event")
	local kind = text(raw.type, "stream event.type")
	if kind == "init" then
		session(raw.session_id, context, "init event")
		return { type = "run.started", payload = {} }
	end
	if kind == "message" then
		return message(raw)
	end
	if kind == "tool_use" then
		return tool_use(raw)
	end
	if kind == "tool_result" then
		return tool_result(raw)
	end
	if kind == "result" then
		return result(raw)
	end
	return nil
end

function M.new(opts)
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	for key in pairs(opts) do
		if key ~= "event_id" and key ~= "now" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	return setmetatable({
		decoder = decoder.new({ provider = "gemini", decode = decode, event_id = opts.event_id, now = opts.now }),
	}, Stream)
end

function M.unavailable(reason)
	return setmetatable({ decoder = decoder.unavailable({ provider = "gemini", reason = reason }) }, Stream)
end

function M.is(value)
	return getmetatable(value) == Stream
end

function Stream:status()
	if not M.is(self) then
		fail("status requires a Gemini stream")
	end
	return self.decoder:status()
end

function Stream:cancel(reason)
	if not M.is(self) then
		fail("cancel requires a Gemini stream")
	end
	return self.decoder:cancel(reason)
end

function Stream:feed(raw, context)
	if not M.is(self) then
		fail("feed requires a Gemini stream")
	end
	if self.decoder:status().state ~= "ready" then
		return self.decoder:decode(raw, context)
	end
	if decode(raw, context or {}) == nil then
		return {}
	end
	return self.decoder:decode(raw, context)
end

return M
