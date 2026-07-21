local decoder = require("gator.adapters.decoder")
local redact = require("gator.policy.redact")
local M = {}
local Stream = {}

Stream.__index = Stream

local function fail(message)
	error("Gator Claude stream: " .. redact.text(tostring(message)), 3)
end

local function text(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be non-empty text")
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
	value = text(value, name .. ".session_id")
	if context.session_id ~= nil and context.session_id ~= value then
		fail(name .. " belongs to a different provider-owned session")
	end
	return value
end

local function optional_session(value, context, name)
	if value ~= nil then
		session(value, context, name)
	end
end

local function assistant(value, context)
	object(value, "assistant event")
	optional_session(value.session_id, context, "assistant event")
	local message = object(value.message, "assistant event.message")
	if message.role ~= "assistant" then
		fail("assistant event.message.role must be assistant")
	end
	if type(message.content) ~= "table" or not vim.islist(message.content) then
		fail("assistant event.message.content must be an array")
	end
	local events = {}
	for _, content in ipairs(message.content) do
		object(content, "assistant event content")
		if content.type == "text" then
			events[#events + 1] =
				{ type = "message.completed", payload = { text = text(content.text, "assistant text") } }
		end
	end
	return #events > 0 and events or nil
end

local function result(value, context)
	object(value, "result event")
	optional_session(value.session_id, context, "result event")
	local subtype = text(value.subtype, "result event.subtype")
	if type(value.is_error) ~= "boolean" then
		fail("result event.is_error must be a boolean")
	end
	if subtype == "success" and value.is_error == false then
		return { type = "run.completed", payload = { status = "completed" } }
	end
	if value.is_error ~= true then
		fail("result event subtype and error state are inconsistent")
	end
	local message = type(value.result) == "string" and redact.text(value.result) or "Claude stream failed"
	return { type = "run.error", payload = { kind = "provider", message = message, retryable = false } }
end

local function decode(raw, context)
	object(raw, "stream event")
	local kind = text(raw.type, "stream event.type")
	if kind == "system" and raw.subtype == "init" then
		session(raw.session_id, context, "native init")
		return { type = "run.started", payload = {} }
	end
	if kind == "assistant" then
		return assistant(raw, context)
	end
	if kind == "result" then
		return result(raw, context)
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
		decoder = decoder.new({ provider = "claude", decode = decode, event_id = opts.event_id, now = opts.now }),
	}, Stream)
end

function M.unavailable(reason)
	return setmetatable({ decoder = decoder.unavailable({ provider = "claude", reason = reason }) }, Stream)
end

function M.is(value)
	return getmetatable(value) == Stream
end

function Stream:status()
	if not M.is(self) then
		fail("status requires a Claude stream")
	end
	return self.decoder:status()
end

function Stream:cancel(reason)
	if not M.is(self) then
		fail("cancel requires a Claude stream")
	end
	return self.decoder:cancel(reason)
end

function Stream:feed(raw, context)
	if not M.is(self) then
		fail("feed requires a Claude stream")
	end
	local state = self.decoder:status()
	if state.state ~= "ready" then
		return self.decoder:decode(raw, context)
	end
	if decode(raw, context or {}) == nil then
		return {}
	end
	return self.decoder:decode(raw, context)
end

return M
