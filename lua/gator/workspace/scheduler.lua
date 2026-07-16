local M = {}
local Scheduler = {}

Scheduler.__index = Scheduler

local function fail(message)
	error("Gator workspace scheduler: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function snapshot(run)
	return vim.deepcopy({ id = run.id, state = run.state, result = run.result, failure = run.failure })
end

local function dispatch(self)
	if self.dispatching then
		return
	end
	self.dispatching = true
	while self.active < self.maximum and #self.queue > 0 do
		local run = table.remove(self.queue, 1)
		run.state = "running"
		self.active = self.active + 1
		local settled = false
		local function complete(result)
			if settled then
				return false
			end
			if type(result) ~= "boolean" then
				fail("completion result must be boolean")
			end
			settled = true
			self.active = self.active - 1
			run.result = result
			run.state = result and "completed" or "failed"
			dispatch(self)
			return true
		end
		local ok, value = pcall(run.start, snapshot(run), complete)
		if not ok then
			run.failure = "launch failed"
			complete(false)
		elseif value == false then
			run.failure = "launch rejected"
			complete(false)
		end
	end
	self.dispatching = false
end

function M.new(opts)
	if opts == nil then
		opts = {}
	end
	if type(opts) ~= "table" then
		fail("options must provide an optional integer maximum")
	end
	for key in pairs(opts) do
		if key ~= "maximum" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	if
		opts.maximum ~= nil
		and (
			type(opts.maximum) ~= "number"
			or opts.maximum ~= opts.maximum
			or opts.maximum == math.huge
			or opts.maximum % 1 ~= 0
		)
	then
		fail("options must provide an optional integer maximum")
	end
	local maximum = opts.maximum or 1
	if maximum < 1 then
		fail("maximum must be positive")
	end
	return setmetatable({ maximum = maximum, active = 0, queue = {}, runs = {}, dispatching = false }, Scheduler)
end

function Scheduler:submit(opts)
	if type(opts) ~= "table" then
		fail("submit requires options")
	end
	for key in pairs(opts) do
		if key ~= "id" and key ~= "start" then
			fail("submit contains unsupported field: " .. tostring(key))
		end
	end
	local id = identifier(opts.id, "id")
	if self.runs[id] then
		fail("run id is already scheduled: " .. id)
	end
	if type(opts.start) ~= "function" then
		fail("start must be a function")
	end
	local run = { id = id, start = opts.start, state = "queued" }
	self.runs[id] = run
	table.insert(self.queue, run)
	dispatch(self)
	return self:status(id)
end

function Scheduler:status(id)
	local run = self.runs[identifier(id, "id")]
	return run and snapshot(run) or nil
end

function Scheduler:list()
	local result = {}
	for _, run in pairs(self.runs) do
		table.insert(result, snapshot(run))
	end
	table.sort(result, function(left, right)
		return left.id < right.id
	end)
	return result
end

return M
