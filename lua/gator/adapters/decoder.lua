local provider_event = require("gator.core.provider_event")
local redact = require("gator.policy.redact")

local M = { api_version = 1 }
local Decoder = {}

Decoder.__index = Decoder

local function fail(message)
	error("Gator provider decoder: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function string(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty string")
	end
	return value
end

local function fields(value, allowed, name)
	if type(value) ~= "table" then
		fail(name .. " must be a table")
	end
	for key in pairs(value) do
		if not allowed[key] then
			fail(name .. " contains unsupported field: " .. tostring(key))
		end
	end
end

local function context(value)
	fields(value, { run_id = true, session_id = true }, "context")
	local result = { run_id = identifier(value.run_id, "context.run_id") }
	if value.session_id ~= nil then
		result.session_id = string(value.session_id, "context.session_id")
	end
	return result
end

local function parts(value)
	if type(value) ~= "table" then
		fail("decode callback must return an event object or array")
	end
	if vim.islist(value) then
		if #value == 0 then
			fail("decode callback must not return an empty event array")
		end
		return value
	end
	return { value }
end

local function part(value)
	fields(value, { type = true, payload = true, at = true }, "decoded event")
	return {
		type = string(value.type, "decoded event.type"),
		payload = value.payload or {},
		at = value.at,
	}
end

local function status(value)
	if value.available == false then
		return { provider = value.provider, available = false, state = "unavailable", reason = value.reason }
	end
	if value.cancelled then
		return { provider = value.provider, available = true, state = "cancelled", reason = value.reason }
	end
	return { provider = value.provider, available = true, state = "ready" }
end

function M.new(opts)
	fields(opts, { provider = true, decode = true, event_id = true, now = true }, "new")
	local provider = identifier(opts.provider, "provider")
	if type(opts.decode) ~= "function" then
		fail("decode must be a function")
	end
	if opts.event_id ~= nil and type(opts.event_id) ~= "function" then
		fail("event_id must be a function")
	end
	if opts.now ~= nil and type(opts.now) ~= "function" then
		fail("now must be a function")
	end
	return setmetatable({
		provider = provider,
		decode_callback = opts.decode,
		event_id = opts.event_id or function(context)
			return provider .. "-event-" .. context.sequence
		end,
		now = opts.now or os.time,
		sequence = 0,
	}, Decoder)
end

function M.unavailable(opts)
	fields(opts, { provider = true, reason = true }, "unavailable")
	return setmetatable({
		provider = identifier(opts.provider, "provider"),
		available = false,
		reason = redact.text(string(opts.reason, "reason")),
	}, Decoder)
end

function M.is(value)
	return getmetatable(value) == Decoder
end

function Decoder:status()
	if not M.is(self) then
		fail("status requires a decoder")
	end
	return vim.deepcopy(status(self))
end

function Decoder:cancel(reason)
	if not M.is(self) then
		fail("cancel requires a decoder")
	end
	if self.available == false or self.cancelled then
		return false
	end
	if reason ~= nil then
		self.reason = redact.text(string(reason, "cancellation reason"))
	end
	self.cancelled = true
	return true
end

function Decoder:decode(raw, value)
	if not M.is(self) then
		fail("decode requires a decoder")
	end
	if self.available == false then
		fail("decoder is unavailable: " .. self.reason)
	end
	if self.cancelled then
		fail("decoder is cancelled" .. (self.reason and ": " .. self.reason or ""))
	end
	local event_context = context(value)
	local ok, decoded = xpcall(function()
		return self.decode_callback(raw, vim.deepcopy(event_context))
	end, debug.traceback)
	if not ok then
		fail("decode callback failed: " .. redact.text(decoded))
	end
	local result = {}
	for _, value in ipairs(parts(decoded)) do
		value = part(value)
		local sequence = self.sequence
		local id_ok, id = xpcall(function()
			return self.event_id({
				provider = self.provider,
				run_id = event_context.run_id,
				sequence = sequence,
			})
		end, debug.traceback)
		if not id_ok then
			fail("event_id callback failed: " .. redact.text(id))
		end
		local at = value.at
		if at == nil then
			local now_ok
			now_ok, at = xpcall(self.now, debug.traceback)
			if not now_ok then
				fail("now callback failed: " .. redact.text(at))
			end
		end
		result[#result + 1] = provider_event.new({
			schema_version = provider_event.schema_version,
			id = id,
			run_id = event_context.run_id,
			provider = { name = self.provider, session_id = event_context.session_id },
			sequence = sequence,
			type = value.type,
			at = at,
			payload = value.payload,
		})
		self.sequence = sequence + 1
	end
	return result
end

return M
