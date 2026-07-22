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

local function worktree(value)
	if type(value) ~= "string" or value == "" or value:find("\0", 1, true) then
		fail("worktree must be non-empty text without NUL")
	end
	return value
end

local function snapshot(run)
	return vim.deepcopy({
		id = run.id,
		worktree = run.worktree,
		provider = run.provider,
		state = run.state,
		result = run.result,
		failure = run.failure,
	})
end

local function next_run(self)
	for index, run in ipairs(self.queue) do
		if not self.active_worktrees[run.worktree] then
			return table.remove(self.queue, index)
		end
	end
end

local function dispatch(self)
	if self.dispatching then
		return
	end
	self.dispatching = true
	while self.active < self.maximum and #self.queue > 0 do
		local run = next_run(self)
		if not run then
			break
		end
		run.state = "running"
		self.active = self.active + 1
		self.active_worktrees[run.worktree] = run.id
		local settled = false
		local function settle(result, state, failure)
			if settled then
				return false
			end
			if type(result) ~= "boolean" then
				fail("completion result must be boolean")
			end
			settled = true
			self.active = self.active - 1
			self.active_worktrees[run.worktree] = nil
			run.result = result
			run.state = state or (result and "completed" or "failed")
			run.failure = failure or run.failure
			dispatch(self)
			return true
		end
		local function complete(result)
			return settle(result)
		end
		run.settle = settle
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
	return setmetatable({
		maximum = maximum,
		active = 0,
		active_worktrees = {},
		queue = {},
		runs = {},
		dispatching = false,
	}, Scheduler)
end

function Scheduler:submit(opts)
	if type(opts) ~= "table" then
		fail("submit requires options")
	end
	for key in pairs(opts) do
		if key ~= "id" and key ~= "worktree" and key ~= "provider" and key ~= "start" and key ~= "cancel" then
			fail("submit contains unsupported field: " .. tostring(key))
		end
	end
	local id = identifier(opts.id, "id")
	local worktree_name = worktree(opts.worktree)
	local provider = identifier(opts.provider, "provider")
	if self.runs[id] then
		fail("run id is already scheduled: " .. id)
	end
	if type(opts.start) ~= "function" then
		fail("start must be a function")
	end
	if opts.cancel ~= nil and type(opts.cancel) ~= "function" then
		fail("cancel must be a function")
	end
	local run = {
		id = id,
		worktree = worktree_name,
		provider = provider,
		start = opts.start,
		cancel = opts.cancel,
		state = "queued",
	}
	self.runs[id] = run
	table.insert(self.queue, run)
	dispatch(self)
	return self:status(id)
end

function Scheduler:cancel(id)
	local run = self.runs[identifier(id, "id")]
	if not run then
		return { id = id, state = "unavailable", failure = "run is unavailable" }
	end
	if run.state == "queued" then
		for index, candidate in ipairs(self.queue) do
			if candidate == run then
				table.remove(self.queue, index)
				break
			end
		end
		run.state = "cancelled"
		run.failure = "cancelled"
		return snapshot(run)
	end
	if run.state ~= "running" then
		return snapshot(run)
	end
	if not run.cancel then
		return { id = id, state = "unavailable", failure = "cancellation is unavailable" }
	end
	local ok, cancelled = pcall(run.cancel, snapshot(run))
	if not ok or cancelled ~= true then
		return { id = id, state = "failed", failure = "cancellation failed" }
	end
	run.settle(false, "cancelled", "cancelled")
	return snapshot(run)
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
