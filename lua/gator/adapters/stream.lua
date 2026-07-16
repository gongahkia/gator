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

function Parser:finish()
	if self.buffer ~= "" then
		fail("stream ended with a partial JSONL record")
	end
	if not self.completed then
		fail("stream ended before a completion event")
	end
	return true
end

return M
