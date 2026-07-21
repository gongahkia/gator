local decoder = require("gator.adapters.decoder")
local M = {}
local Stream = {}

Stream.__index = Stream

local function fail(message)
	error("Gator Codex stream: " .. message, 3)
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

local function text(value, name)
	if type(value) ~= "string" then
		fail(name .. " must be a string")
	end
	return value
end

local function integer(value, name)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail(name .. " must be a non-negative integer")
	end
	return value
end

local function thread(value, context, name)
	value = string(value, name .. ".threadId")
	if context.session_id ~= nil and context.session_id ~= value then
		fail(name .. " belongs to a different provider-owned thread")
	end
	return value
end

local function turn(value, name)
	fields(value, { id = true, items = true, status = true }, name)
	string(value.id, name .. ".id")
	if type(value.items) ~= "table" or not vim.islist(value.items) then
		fail(name .. ".items must be an array")
	end
	if
		value.status ~= "inProgress"
		and value.status ~= "completed"
		and value.status ~= "interrupted"
		and value.status ~= "failed"
	then
		fail(name .. ".status is unsupported")
	end
	return value
end

local function item(value, name)
	if type(value) ~= "table" or vim.islist(value) then
		fail(name .. " must be an object")
	end
	string(value.id, name .. ".id")
	string(value.type, name .. ".type")
	if value.type == "agentMessage" then
		text(value.text, name .. ".text")
	end
	return value
end

local function usage(value)
	fields(value, {
		cachedInputTokens = true,
		inputTokens = true,
		outputTokens = true,
		reasoningOutputTokens = true,
		totalTokens = true,
	}, "token usage")
	for _, name in ipairs({ "cachedInputTokens", "inputTokens", "outputTokens", "reasoningOutputTokens", "totalTokens" }) do
		integer(value[name], "token usage." .. name)
	end
	return value
end

local function path(value)
	value = string(value, "file change path")
	if value:sub(1, 1) == "/" or value:match("^%a:[/\\]") or value:find("..", 1, true) then
		fail("file change path must be a relative repository path")
	end
	return value
end

local function file_change(value)
	fields(value, { diff = true, kind = true, path = true }, "file change")
	if type(value.diff) ~= "string" then
		fail("file change.diff must be a string")
	end
	fields(value.kind, { type = true, move_path = true }, "file change.kind")
	local kinds = { add = "created", delete = "deleted", update = "modified" }
	local kind = kinds[value.kind.type]
	if not kind then
		fail("file change.kind.type is unsupported")
	end
	if value.kind.move_path ~= nil and type(value.kind.move_path) ~= "string" then
		fail("file change.kind.move_path must be a string")
	end
	return { type = "file.change", payload = { path = path(value.path), kind = kind } }
end

local function decode(raw, context)
	if type(raw) ~= "table" or vim.islist(raw) then
		fail("record must be an object")
	end
	if raw.method == nil then
		if raw.id == nil then
			fail("record must be a JSON-RPC response or notification")
		end
		return nil
	end
	fields(raw, { method = true, params = true }, "notification")
	local method = string(raw.method, "notification.method")
	if method == "thread/tokenUsage/updated" then
		fields(raw.params, { threadId = true, tokenUsage = true, turnId = true }, method .. " params")
		thread(raw.params.threadId, context, method .. " params")
		string(raw.params.turnId, method .. " params.turnId")
		fields(
			raw.params.tokenUsage,
			{ last = true, modelContextWindow = true, total = true },
			method .. " params.tokenUsage"
		)
		local value = usage(raw.params.tokenUsage.last)
		usage(raw.params.tokenUsage.total)
		if raw.params.tokenUsage.modelContextWindow ~= nil and raw.params.tokenUsage.modelContextWindow ~= vim.NIL then
			integer(raw.params.tokenUsage.modelContextWindow, method .. " params.tokenUsage.modelContextWindow")
		end
		return {
			type = "usage.update",
			payload = { input = value.inputTokens, output = value.outputTokens, total = value.totalTokens },
		}
	end
	if method == "item/fileChange/patchUpdated" then
		fields(raw.params, { changes = true, itemId = true, threadId = true, turnId = true }, method .. " params")
		thread(raw.params.threadId, context, method .. " params")
		string(raw.params.turnId, method .. " params.turnId")
		string(raw.params.itemId, method .. " params.itemId")
		if type(raw.params.changes) ~= "table" or not vim.islist(raw.params.changes) or #raw.params.changes == 0 then
			fail(method .. " params.changes must be a non-empty array")
		end
		local events = {}
		for index, value in ipairs(raw.params.changes) do
			events[index] = file_change(value)
		end
		return events
	end
	if method == "thread/compacted" then
		fields(raw.params, { threadId = true, turnId = true }, method .. " params")
		thread(raw.params.threadId, context, method .. " params")
		string(raw.params.turnId, method .. " params.turnId")
		return { type = "context.compacted", payload = {} }
	end
	if method == "turn/started" or method == "turn/completed" then
		fields(raw.params, { threadId = true, turn = true }, method .. " params")
		thread(raw.params.threadId, context, method .. " params")
		local value = turn(raw.params.turn, method .. " params.turn")
		if method == "turn/started" then
			if value.status ~= "inProgress" then
				fail("turn/started params.turn.status must be inProgress")
			end
			return { type = "run.started", payload = {} }
		end
		if value.status == "inProgress" then
			fail("turn/completed params.turn.status cannot be inProgress")
		end
		return { type = "run.completed", payload = { status = value.status } }
	end
	if method == "item/started" or method == "item/completed" then
		local suffix = method == "item/started" and "startedAtMs" or "completedAtMs"
		fields(raw.params, { threadId = true, turnId = true, item = true, [suffix] = true }, method .. " params")
		thread(raw.params.threadId, context, method .. " params")
		string(raw.params.turnId, method .. " params.turnId")
		integer(raw.params[suffix], method .. " params." .. suffix)
		local value = item(raw.params.item, method .. " params.item")
		if value.type == "contextCompaction" then
			return {
				type = method == "item/started" and "context.compaction_started" or "context.compacted",
				payload = {},
			}
		end
		if value.type ~= "agentMessage" then
			return nil
		end
		return {
			type = method == "item/started" and "message.started" or "message.completed",
			payload = { text = value.text },
		}
	end
	if method == "item/agentMessage/delta" then
		fields(raw.params, { threadId = true, turnId = true, itemId = true, delta = true }, method .. " params")
		thread(raw.params.threadId, context, method .. " params")
		string(raw.params.turnId, method .. " params.turnId")
		string(raw.params.itemId, method .. " params.itemId")
		return { type = "message.delta", payload = { text = text(raw.params.delta, method .. " params.delta") } }
	end
	if method == "error" then
		fields(raw.params, { error = true, threadId = true, turnId = true, willRetry = true }, "error params")
		thread(raw.params.threadId, context, "error params")
		string(raw.params.turnId, "error params.turnId")
		if type(raw.params.willRetry) ~= "boolean" then
			fail("error params.willRetry must be a boolean")
		end
		fields(
			raw.params.error,
			{ message = true, additionalDetails = true, codexErrorInfo = true },
			"error params.error"
		)
		return {
			type = "run.error",
			payload = {
				kind = "provider",
				message = text(raw.params.error.message, "error params.error.message"),
				retryable = raw.params.willRetry,
			},
		}
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
		decoder = decoder.new({ provider = "codex", decode = decode, event_id = opts.event_id, now = opts.now }),
	}, Stream)
end

function M.unavailable(reason)
	return setmetatable({ decoder = decoder.unavailable({ provider = "codex", reason = reason }) }, Stream)
end

function M.is(value)
	return getmetatable(value) == Stream
end

function Stream:status()
	if not M.is(self) then
		fail("status requires a Codex stream")
	end
	return self.decoder:status()
end

function Stream:cancel(reason)
	if not M.is(self) then
		fail("cancel requires a Codex stream")
	end
	return self.decoder:cancel(reason)
end

function Stream:feed(raw, context)
	if not M.is(self) then
		fail("feed requires a Codex stream")
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
