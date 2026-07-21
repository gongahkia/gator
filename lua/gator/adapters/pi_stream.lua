local decoder = require("gator.adapters.decoder")
local M = {}
local Stream = {}

Stream.__index = Stream

local function fail(message)
	error("Gator Pi stream: " .. message, 3)
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

local function text(message, name)
	fields(message, { role = true, content = true }, name)
	if message.role ~= "assistant" then
		return nil
	end
	if type(message.content) ~= "table" or not vim.islist(message.content) then
		fail(name .. ".content must be an array")
	end
	local parts = {}
	for index, part in ipairs(message.content) do
		if type(part) ~= "table" or vim.islist(part) or type(part.type) ~= "string" then
			fail(name .. ".content[" .. index .. "] must be an object with a type")
		end
		if part.type == "text" then
			if type(part.text) ~= "string" then
				fail(name .. ".content[" .. index .. "].text must be a string")
			end
			table.insert(parts, part.text)
		end
	end
	return table.concat(parts)
end

local function decode(raw)
	if type(raw) ~= "table" or vim.islist(raw) or type(raw.type) ~= "string" or raw.type == "" then
		fail("record must be an object with a non-empty type")
	end
	if raw.type == "response" then
		return nil
	end
	if raw.type == "agent_start" then
		fields(raw, { type = true }, "agent_start")
		return { type = "run.started", payload = {} }
	end
	if raw.type == "agent_settled" then
		fields(raw, { type = true }, "agent_settled")
		return { type = "run.settled", payload = {} }
	end
	if raw.type == "agent_end" then
		fields(raw, { type = true, messages = true, willRetry = true }, "agent_end")
		if type(raw.messages) ~= "table" or not vim.islist(raw.messages) then
			fail("agent_end.messages must be an array")
		end
		if raw.willRetry ~= nil and type(raw.willRetry) ~= "boolean" then
			fail("agent_end.willRetry must be a boolean")
		end
		return {
			type = "run.completed",
			payload = { message_count = #raw.messages, will_retry = raw.willRetry == true },
		}
	end
	if raw.type ~= "message_start" and raw.type ~= "message_update" and raw.type ~= "message_end" then
		return nil
	end
	fields(raw, { type = true, message = true, assistantMessageEvent = true }, raw.type)
	local value = text(raw.message, raw.type .. ".message")
	if value == nil then
		return nil
	end
	local action = raw.type == "message_start" and "started" or raw.type == "message_update" and "delta" or "completed"
	return { type = "message." .. action, payload = { text = value } }
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
		decoder = decoder.new({ provider = "pi", decode = decode, event_id = opts.event_id, now = opts.now }),
	}, Stream)
end

function M.unavailable(reason)
	return setmetatable({ decoder = decoder.unavailable({ provider = "pi", reason = reason }) }, Stream)
end

function M.is(value)
	return getmetatable(value) == Stream
end

function Stream:status()
	if not M.is(self) then
		fail("status requires a Pi stream")
	end
	return self.decoder:status()
end

function Stream:cancel(reason)
	if not M.is(self) then
		fail("cancel requires a Pi stream")
	end
	return self.decoder:cancel(reason)
end

function Stream:feed(raw, context)
	if not M.is(self) then
		fail("feed requires a Pi stream")
	end
	local state = self.decoder:status()
	if state.state ~= "ready" then
		return self.decoder:decode(raw, context)
	end
	local normalized = decode(raw)
	if normalized == nil then
		return {}
	end
	return self.decoder:decode(raw, context)
end

return M
