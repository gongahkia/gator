local M = {}
local Parser = {}

Parser.__index = Parser

local function fail(message)
	error("Gator JSONL stream: " .. message, 3)
end

local function event(value)
	if type(value) ~= "table" or vim.islist(value) or type(value.type) ~= "string" or value.type == "" then
		fail("event must be an object with a non-empty type")
	end
	local data = {}
	for key, item in pairs(value) do
		if key ~= "type" then
			data[key] = vim.deepcopy(item)
		end
	end
	return { type = value.type, data = data }
end

function M.new(opts)
	if type(opts) ~= "table" or type(opts.on_event) ~= "function" or type(opts.on_complete) ~= "function" then
		fail("new requires on_event and on_complete callbacks")
	end
	if opts.on_error ~= nil and type(opts.on_error) ~= "function" then
		fail("on_error must be a function")
	end
	return setmetatable({
		on_event = opts.on_event,
		on_complete = opts.on_complete,
		on_error = opts.on_error,
		buffer = "",
		completed = false,
	}, Parser)
end

local function parse(parser, line)
	local ok, value = pcall(vim.json.decode, line)
	if not ok then
		return false, "invalid JSON"
	end
	local event_ok, normalized = pcall(event, value)
	if not event_ok then
		return false, normalized
	end
	if parser.completed then
		return false, "event received after completion"
	end
	if normalized.type == "complete" then
		parser.completed = true
		parser.on_complete(normalized)
	else
		parser.on_event(normalized)
	end
	return true
end

function Parser:feed(chunk)
	if type(chunk) ~= "string" or chunk == "" then
		fail("chunk must be a non-empty string")
	end
	if self.pending then
		fail("cannot synchronously feed a scheduled stream")
	end
	self.buffer = self.buffer .. chunk
	local malformed = 0
	while true do
		local ending = self.buffer:find("\n", 1, true)
		if not ending then
			return malformed
		end
		local line = self.buffer:sub(1, ending - 1):gsub("\r$", "")
		self.buffer = self.buffer:sub(ending + 1)
		local ok, reason
		if line == "" then
			ok, reason = false, "blank JSONL record"
		else
			ok, reason = parse(self, line)
		end
		if not ok then
			malformed = malformed + 1
			if self.on_error then
				self.on_error(reason, line)
			else
				fail(reason)
			end
		end
	end
end

function Parser:feed_async(chunk, opts)
	if type(chunk) ~= "string" or chunk == "" then
		fail("chunk must be a non-empty string")
	end
	if self.pending then
		fail("stream drain is already scheduled")
	end
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("async options must be a table")
	end
	for key in pairs(opts) do
		if key ~= "schedule" and key ~= "batch_size" and key ~= "on_drain" then
			fail("async options contain unsupported field: " .. tostring(key))
		end
	end
	local schedule = opts.schedule or vim.schedule
	if type(schedule) ~= "function" then
		fail("schedule must be a function")
	end
	local batch_size = opts.batch_size or 100
	if type(batch_size) ~= "number" or batch_size < 1 or batch_size % 1 ~= 0 then
		fail("batch_size must be a positive integer")
	end
	if opts.on_drain ~= nil and type(opts.on_drain) ~= "function" then
		fail("on_drain must be a function")
	end
	self.buffer = self.buffer .. chunk
	self.pending = true
	local malformed = 0
	local function drain()
		for _ = 1, batch_size do
			local ending = self.buffer:find("\n", 1, true)
			if not ending then
				self.pending = false
				if opts.on_drain then
					opts.on_drain(malformed)
				end
				return
			end
			local line = self.buffer:sub(1, ending - 1):gsub("\r$", "")
			self.buffer = self.buffer:sub(ending + 1)
			local ok, reason
			if line == "" then
				ok, reason = false, "blank JSONL record"
			else
				ok, reason = parse(self, line)
			end
			if not ok then
				malformed = malformed + 1
				if self.on_error then
					self.on_error(reason, line)
				else
					self.pending = false
					fail(reason)
				end
			end
		end
		schedule(drain)
	end
	schedule(drain)
	return true
end

function Parser:finish()
	if self.pending then
		fail("stream drain is still scheduled")
	end
	if self.buffer ~= "" then
		fail("stream ended with a partial JSONL record")
	end
	if not self.completed then
		fail("stream ended before a completion event")
	end
	return true
end

return M
