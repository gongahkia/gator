local redact = require("gator.policy.redact")

local M = {
	api_version = 1,
	default_limit = 4096,
	states = { ready = true, unavailable = true, failed = true, cancelled = true },
}
local Excerpt = {}

Excerpt.__index = Excerpt

local function fail(message)
	error("Gator output excerpt: " .. redact.text(tostring(message)), 3)
end

local function limit(value)
	if type(value) ~= "number" or value < 1 or value % 1 ~= 0 then
		fail("limit must be a positive integer")
	end
	return value
end

local function reason(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be non-empty text")
	end
	return redact.text(value)
end

function M.new(opts)
	if opts == nil then
		opts = {}
	end
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	for key in pairs(opts) do
		if key ~= "limit" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	return setmetatable({
		state = "ready",
		limit = limit(opts.limit or M.default_limit),
		text = "",
		truncated = false,
		dropped_bytes = 0,
	}, Excerpt)
end

function M.unavailable(opts)
	if type(opts) ~= "table" then
		fail("unavailable requires options")
	end
	for key in pairs(opts) do
		if key ~= "reason" and key ~= "limit" then
			fail("unavailable contains unsupported field: " .. tostring(key))
		end
	end
	return setmetatable({
		state = "unavailable",
		limit = limit(opts.limit or M.default_limit),
		text = "",
		truncated = false,
		dropped_bytes = 0,
		reason = reason(opts.reason, "reason"),
	}, Excerpt)
end

function M.is(value)
	return getmetatable(value) == Excerpt
end

function Excerpt:status()
	if not M.is(self) then
		fail("status requires an output excerpt")
	end
	return vim.deepcopy({
		available = self.state ~= "unavailable",
		state = self.state,
		limit = self.limit,
		bytes = #self.text,
		text = self.text,
		truncated = self.truncated,
		dropped_bytes = self.dropped_bytes,
		reason = self.reason,
	})
end

function Excerpt:append(chunk)
	if not M.is(self) then
		fail("append requires an output excerpt")
	end
	if self.state ~= "ready" then
		return self:status()
	end
	if type(chunk) ~= "string" or chunk == "" then
		self.state = "failed"
		self.reason = "chunk must be non-empty text"
		return self:status()
	end
	local safe = redact.text(chunk)
	local text = self.text .. safe
	if #text > self.limit then
		local dropped = #text - self.limit
		self.text = text:sub(dropped + 1)
		self.truncated = true
		self.dropped_bytes = self.dropped_bytes + dropped
	else
		self.text = text
	end
	return self:status()
end

function Excerpt:cancel(value)
	if not M.is(self) then
		fail("cancel requires an output excerpt")
	end
	if self.state ~= "ready" then
		return false
	end
	self.state = "cancelled"
	if value ~= nil then
		self.reason = reason(value, "cancellation reason")
	end
	return true
end

return M
