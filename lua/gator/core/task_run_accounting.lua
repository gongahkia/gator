local redact = require("gator.policy.redact")
local run = require("gator.core.run")

local M = { api_version = 1 }
local Account = {}

Account.__index = Account

local function fail(message)
	error("Gator task run accounting: " .. redact.text(tostring(message)), 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function terminal(value)
	return value == "completed" or value == "failed" or value == "cancelled"
end

local function summary(self, task_id)
	task_id = identifier(task_id, "task_id")
	local value = {
		task_id = task_id,
		maximum = self.maximum,
		queued = 0,
		running = 0,
		completed = 0,
		failed = 0,
		cancelled = 0,
	}
	local known = false
	for _, record in pairs(self.runs) do
		if record.task_id == task_id then
			known = true
			value[record.state] = value[record.state] + 1
		end
	end
	if not known then
		value.state = "unavailable"
		value.available_slots = self.maximum
		return value
	end
	value.active = value.running
	value.available_slots = math.max(0, self.maximum - value.running)
	value.state = value.running >= self.maximum and "saturated" or "available"
	return value
end

function M.new(opts)
	if opts == nil then
		opts = {}
	end
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	for key in pairs(opts) do
		if key ~= "maximum" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	local maximum = opts.maximum or 1
	if type(maximum) ~= "number" or maximum < 1 or maximum % 1 ~= 0 then
		fail("maximum must be a positive integer")
	end
	return setmetatable({ maximum = maximum, runs = {} }, Account)
end

function M.is(value)
	return getmetatable(value) == Account
end

function Account:observe(value)
	if not M.is(self) then
		fail("observe requires task run accounting")
	end
	if not run.is(value) then
		fail("observe requires a persistent Gator run")
	end
	local record = run.to_record(value)
	local previous = self.runs[record.id]
	if previous and previous.task_id ~= record.task_id then
		fail("run cannot move between tasks")
	end
	self.runs[record.id] = { task_id = record.task_id, state = record.state }
	return vim.deepcopy(summary(self, record.task_id))
end

function Account:status(task_id)
	if not M.is(self) then
		fail("status requires task run accounting")
	end
	return vim.deepcopy(summary(self, task_id))
end

function Account:can_start(task_id)
	if not M.is(self) then
		fail("can_start requires task run accounting")
	end
	local value = summary(self, task_id)
	return { allowed = value.state == "available", status = vim.deepcopy(value) }
end

function Account:cancel(id)
	if not M.is(self) then
		fail("cancel requires task run accounting")
	end
	id = identifier(id, "run id")
	local value = self.runs[id]
	if not value then
		return { state = "unavailable", run_id = id }
	end
	if terminal(value.state) then
		return { state = value.state, run_id = id, changed = false }
	end
	value.state = "cancelled"
	return { state = "cancelled", run_id = id, changed = true, task = vim.deepcopy(summary(self, value.task_id)) }
end

function Account:list()
	if not M.is(self) then
		fail("list requires task run accounting")
	end
	local task_ids = {}
	for _, value in pairs(self.runs) do
		task_ids[value.task_id] = true
	end
	local result = {}
	for task_id in pairs(task_ids) do
		table.insert(result, summary(self, task_id))
	end
	table.sort(result, function(left, right)
		return left.task_id < right.task_id
	end)
	return vim.deepcopy(result)
end

return M
