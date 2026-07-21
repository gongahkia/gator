local decoder = require("gator.adapters.decoder")
local redact = require("gator.policy.redact")
local M = {}
local Stream = {}

Stream.__index = Stream

local function fail(message)
	error("Gator OpenCode stream: " .. redact.text(tostring(message)), 3)
end

local function string(value, name)
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

local function integer(value, name)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail(name .. " must be a non-negative integer")
	end
	return value
end

local function session(value, context)
	value = string(value, "session update.sessionId")
	if context.session_id ~= nil and context.session_id ~= value then
		fail("session update belongs to a different provider-owned session")
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

local function path(value)
	if
		type(value) ~= "string"
		or value == ""
		or value:sub(1, 1) == "/"
		or value:match("^%a:[/\\]")
		or value:find("..", 1, true)
	then
		return nil
	end
	return value
end

local function content(update, action)
	local value = object(update.content, action .. ".content")
	if value.type ~= "text" then
		fail(action .. ".content.type must be text")
	end
	return string(value.text, action .. ".content.text")
end

local function message(update)
	return { type = "message.delta", payload = { text = content(update, "agent_message_chunk") } }
end

local function thought(update)
	return { type = "message.thought", payload = { text = content(update, "agent_thought_chunk") } }
end

local function call(update, calls, commit)
	local id = string(update.toolCallId, "tool_call.toolCallId")
	local title = string(update.title, "tool_call.title")
	local kind = string(update.kind, "tool_call.kind")
	if update.status ~= "pending" then
		fail("tool_call.status must be pending")
	end
	local raw_input = update.rawInput == nil and {} or update.rawInput
	if type(raw_input) ~= "table" or (vim.islist(raw_input) and #raw_input > 0) then
		fail("tool_call.rawInput must be an object")
	end
	local input = safe(raw_input, "tool_call.rawInput")
	if commit then
		calls[id] = { kind = kind, path = path(input.filePath) or path(input.filepath), title = title }
	end
	return { type = "tool.call", payload = { call_id = id, name = title, kind = kind, input = input } }
end

local function result(update, calls, commit)
	local id = string(update.toolCallId, "tool_call_update.toolCallId")
	local states = { completed = "completed", failed = "failed", cancelled = "cancelled", canceled = "cancelled" }
	local state = states[string(update.status, "tool_call_update.status")]
	if not state then
		fail("tool_call_update.status is unsupported")
	end
	local current = calls[id]
	local events = {
		type = "tool.result",
		payload = {
			call_id = id,
			state = state,
			output = update.rawOutput == nil and {} or safe(update.rawOutput, "tool_call_update.rawOutput"),
		},
	}
	if current and current.kind == "edit" and current.path and state == "completed" then
		events = { events, { type = "file.change", payload = { path = current.path, kind = "modified" } } }
	end
	if commit then
		calls[id] = nil
	end
	return events
end

local function progress(update)
	local id = string(update.toolCallId, "tool_call_update.toolCallId")
	if update.status ~= "in_progress" then
		fail("tool_call_update.status is unsupported")
	end
	return { type = "tool.progress", payload = { call_id = id, state = "running" } }
end

local function usage(update)
	local payload = {
		used = integer(update.used, "usage_update.used"),
		size = integer(update.size, "usage_update.size"),
	}
	if update.cost ~= nil then
		local cost = object(update.cost, "usage_update.cost")
		if type(cost.amount) ~= "number" or type(cost.currency) ~= "string" or cost.currency == "" then
			fail("usage_update.cost must contain amount and currency")
		end
		payload.cost = { amount = cost.amount, currency = cost.currency }
	end
	return { type = "usage.update", payload = payload }
end

local function update(value, context, calls, commit)
	object(value, "session update")
	session(value.sessionId, context)
	local body = object(value.update, "session update.update")
	local kind = string(body.sessionUpdate, "session update.update.sessionUpdate")
	if kind == "agent_message_chunk" then
		return message(body)
	end
	if kind == "agent_thought_chunk" then
		return thought(body)
	end
	if kind == "tool_call" then
		return call(body, calls, commit)
	end
	if kind == "tool_call_update" then
		if body.status == "in_progress" then
			return progress(body)
		end
		return result(body, calls, commit)
	end
	if kind == "usage_update" then
		return usage(body)
	end
	return nil
end

local function failure(value)
	local error = object(value.error, "JSON-RPC error")
	if type(error.code) ~= "number" or error.code % 1 ~= 0 then
		fail("JSON-RPC error.code must be an integer")
	end
	return {
		type = "run.error",
		payload = {
			kind = "provider",
			message = redact.text(string(error.message, "JSON-RPC error.message")),
			retryable = false,
		},
	}
end

local function decode(raw, context, calls, commit)
	object(raw, "stream record")
	if raw.jsonrpc ~= "2.0" then
		fail("stream record.jsonrpc must be 2.0")
	end
	if raw.error ~= nil then
		return failure(raw)
	end
	if raw.method ~= "session/update" then
		return nil
	end
	return update(raw.params, context, calls, commit)
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
	local value = setmetatable({ tool_calls = {} }, Stream)
	value.decoder = decoder.new({
		provider = "opencode",
		decode = function(raw, context)
			return decode(raw, context, value.tool_calls, true)
		end,
		event_id = opts.event_id,
		now = opts.now,
	})
	return value
end

function M.signals()
	return {
		provider = "opencode",
		usage = { available = true },
		file_changes = { available = true },
		compaction = { available = false, reason = "OpenCode ACP does not emit compaction updates" },
	}
end

function M.unavailable(reason)
	return setmetatable({ decoder = decoder.unavailable({ provider = "opencode", reason = reason }) }, Stream)
end

function M.is(value)
	return getmetatable(value) == Stream
end

function Stream:status()
	if not M.is(self) then
		fail("status requires an OpenCode stream")
	end
	return self.decoder:status()
end

function Stream:cancel(reason)
	if not M.is(self) then
		fail("cancel requires an OpenCode stream")
	end
	return self.decoder:cancel(reason)
end

function Stream:feed(raw, context)
	if not M.is(self) then
		fail("feed requires an OpenCode stream")
	end
	if self.decoder:status().state ~= "ready" then
		return self.decoder:decode(raw, context)
	end
	if decode(raw, context or {}, self.tool_calls, false) == nil then
		return {}
	end
	return self.decoder:decode(raw, context)
end

return M
