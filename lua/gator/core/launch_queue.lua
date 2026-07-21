local redact = require("gator.policy.redact")
local runtime = require("gator.core.runtime")

local M = {}
local Queue = {}

Queue.__index = Queue

local terminal = { completed = true, failed = true, cancelled = true }

local function fail(message)
	error("Gator launch queue: " .. redact.text(tostring(message)), 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function provider(value)
	if type(value) ~= "string" or value == "" or redact.text(value) ~= value then
		fail("provider must be credential-free text")
	end
	return value
end

local function positive(value, name)
	if type(value) ~= "number" or value < 1 or value % 1 ~= 0 then
		fail(name .. " must be a positive integer")
	end
	return value
end

local function status(value)
	return vim.deepcopy({ id = value.id, provider = value.provider, state = value.state })
end

local function remove(values, target)
	for index, value in ipairs(values) do
		if value == target then
			table.remove(values, index)
			return true
		end
	end
	return false
end

function M.new(opts)
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	for key in pairs(opts) do
		if key ~= "runtime" and key ~= "id" and key ~= "limit" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	if not runtime.is(opts.runtime) then
		fail("new requires a runtime owner")
	end
	local value = setmetatable({
		runtime = opts.runtime,
		id = identifier(opts.id or "provider-launch", "service id"),
		limit = positive(opts.limit or 1, "limit"),
		items = {},
		pending = {},
		running = 0,
		active = false,
		pumping = false,
	}, Queue)
	value.runtime:register(value.id, {
		start = function()
			value.active = true
			value:pump()
			return value
		end,
		stop = function(_, _, reason)
			value:deactivate(reason)
		end,
	})
	return value
end

function M.is(value)
	return getmetatable(value) == Queue
end

function Queue:status(id)
	if not M.is(self) then
		fail("status requires a launch queue")
	end
	id = identifier(id, "launch id")
	return self.items[id] and status(self.items[id]) or nil
end

function Queue:list()
	if not M.is(self) then
		fail("list requires a launch queue")
	end
	local result = {}
	for _, value in ipairs(self.items) do
		table.insert(result, status(value))
	end
	return result
end

function Queue:start()
	if not M.is(self) then
		fail("start requires a launch queue")
	end
	return self.runtime:start(self.id)
end

function Queue:stop(reason)
	if not M.is(self) then
		fail("stop requires a launch queue")
	end
	return self.runtime:stop(self.id, reason or "cancelled")
end

function Queue:pump()
	if not M.is(self) then
		fail("pump requires a launch queue")
	end
	if self.pumping then
		return
	end
	self.pumping = true
	while self.active and self.running < self.limit and #self.pending > 0 do
		local value = table.remove(self.pending, 1)
		if value.state == "queued" then
			value.state = "running"
			self.running = self.running + 1
			local settled = false
			local function complete(result)
				if settled then
					return false
				end
				if type(result) ~= "string" or not terminal[result] then
					fail("launch completion must be completed, failed, or cancelled")
				end
				settled = true
				value.state = result
				value.handle = nil
				self.running = self.running - 1
				self:pump()
				return status(value)
			end
			local ok, handle = pcall(value.launch, complete)
			if not ok then
				value.failure = redact.text(tostring(handle))
				complete("failed")
			elseif not settled then
				value.handle = handle
			end
		end
	end
	self.pumping = false
end

function Queue:enqueue(opts)
	if not M.is(self) then
		fail("enqueue requires a launch queue")
	end
	if not self.active then
		fail("launch queue is unavailable until its runtime service starts")
	end
	if type(opts) ~= "table" then
		fail("enqueue requires options")
	end
	for key in pairs(opts) do
		if key ~= "id" and key ~= "provider" and key ~= "launch" and key ~= "cancel" then
			fail("enqueue contains unsupported field: " .. tostring(key))
		end
	end
	local id = identifier(opts.id, "launch id")
	if self.items[id] then
		fail("launch is already tracked: " .. id)
	end
	if type(opts.launch) ~= "function" or type(opts.cancel) ~= "function" then
		fail("enqueue requires launch and cancel callbacks")
	end
	local value =
		{ id = id, provider = provider(opts.provider), launch = opts.launch, cancel = opts.cancel, state = "queued" }
	self.items[id] = value
	table.insert(self.items, value)
	table.insert(self.pending, value)
	self:pump()
	return status(value)
end

function Queue:cancel(id, reason)
	if not M.is(self) then
		fail("cancel requires a launch queue")
	end
	id = identifier(id, "launch id")
	local value = self.items[id]
	if not value then
		fail("launch is unavailable: " .. id)
	end
	if value.state == "queued" then
		remove(self.pending, value)
		value.state = "cancelled"
		return status(value)
	end
	if value.state ~= "running" then
		return false
	end
	if type(reason) ~= "string" or reason == "" then
		fail("cancel reason must be non-empty text")
	end
	value.state = "cancelling"
	local ok, result = pcall(value.cancel, value.handle, redact.text(reason))
	if not ok or result == false then
		value.state = "running"
		value.failure = redact.text(tostring(result))
		fail("launch cancellation failed: " .. id)
	end
	return status(value)
end

function Queue:deactivate(reason)
	if not M.is(self) then
		fail("deactivate requires a launch queue")
	end
	self.active = false
	for _, value in ipairs(vim.deepcopy(self.pending)) do
		if value.state == "queued" then
			self:cancel(value.id, reason)
		end
	end
	for _, value in ipairs(self.items) do
		if value.state == "running" then
			self:cancel(value.id, reason)
		end
	end
end

return M
