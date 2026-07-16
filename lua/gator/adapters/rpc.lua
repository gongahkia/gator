local M = {}
local Transport = {}

Transport.__index = Transport

local function fail(message)
	error("Gator JSON-RPC: " .. message, 3)
end

local function method(value)
	if type(value) ~= "string" or value == "" then
		fail("method must be a non-empty string")
	end
	return value
end

local function validate_message(value)
	if type(value) ~= "table" or value.jsonrpc ~= "2.0" then
		fail("message must declare jsonrpc 2.0")
	end
	if value.method ~= nil then
		method(value.method)
		if value.result ~= nil or value.error ~= nil then
			fail("requests and notifications cannot include a response")
		end
		return "request"
	end
	if
		value.id == nil
		or (value.result == nil and value.error == nil)
		or (value.result ~= nil and value.error ~= nil)
	then
		fail("responses must include an id and exactly one result or error")
	end
	if
		value.error ~= nil
		and (
			type(value.error) ~= "table"
			or type(value.error.code) ~= "number"
			or type(value.error.message) ~= "string"
		)
	then
		fail("response errors must include numeric code and string message")
	end
	return "response"
end

local function frame(write, value)
	write(vim.json.encode(value) .. "\n")
end

function M.new(opts)
	if type(opts) ~= "table" or type(opts.write) ~= "function" then
		fail("new requires a write function")
	end
	if opts.on_notification ~= nil and type(opts.on_notification) ~= "function" then
		fail("on_notification must be a function")
	end
	if opts.now ~= nil and type(opts.now) ~= "function" then
		fail("now must be a function")
	end
	return setmetatable({
		write = opts.write,
		on_notification = opts.on_notification,
		now = opts.now or function()
			return vim.uv.now()
		end,
		buffer = "",
		next_id = 1,
		pending = {},
	}, Transport)
end

function Transport:request(name, params, callback, timeout_ms)
	method(name)
	if type(callback) ~= "function" then
		fail("request callback must be a function")
	end
	if type(timeout_ms) ~= "number" or timeout_ms < 1 or timeout_ms % 1 ~= 0 then
		fail("timeout_ms must be a positive integer")
	end
	local id = self.next_id
	self.next_id = id + 1
	self.pending[id] = { callback = callback, deadline = self.now() + timeout_ms }
	frame(self.write, { jsonrpc = "2.0", id = id, method = name, params = params })
	return id
end

function Transport:notify(name, params)
	method(name)
	frame(self.write, { jsonrpc = "2.0", method = name, params = params })
end

function Transport:cancel(id)
	local pending = self.pending[id]
	if not pending then
		fail("request is not pending: " .. tostring(id))
	end
	self.pending[id] = nil
	frame(self.write, { jsonrpc = "2.0", method = "$/cancelRequest", params = { id = id } })
	pending.callback(nil, { code = -32800, message = "request cancelled" })
end

function Transport:expire()
	local expired = {}
	local now = self.now()
	for id, pending in pairs(self.pending) do
		if pending.deadline <= now then
			table.insert(expired, id)
		end
	end
	for _, id in ipairs(expired) do
		local pending = self.pending[id]
		self.pending[id] = nil
		pending.callback(nil, { code = -32001, message = "request timed out" })
	end
	return #expired
end

function Transport:feed(chunk)
	if type(chunk) ~= "string" or chunk == "" then
		fail("frame chunk must be a non-empty string")
	end
	self.buffer = self.buffer .. chunk
	while true do
		local ending = self.buffer:find("\n", 1, true)
		if not ending then
			return
		end
		local line = self.buffer:sub(1, ending - 1):gsub("\r$", "")
		self.buffer = self.buffer:sub(ending + 1)
		if line == "" then
			fail("blank JSON-RPC frame")
		end
		local ok, value = pcall(vim.json.decode, line)
		if not ok then
			fail("invalid JSON-RPC frame")
		end
		local kind = validate_message(value)
		if kind == "response" then
			local pending = self.pending[value.id]
			if not pending then
				fail("response does not match a pending request")
			end
			self.pending[value.id] = nil
			pending.callback(value.result, value.error)
		elseif value.id == nil and self.on_notification then
			self.on_notification(value.method, value.params)
		end
	end
end

return M
