local redact = require("gator.policy.redact")
local run = require("gator.core.run")

local M = { api_version = 1, states = { ready = true, cancelled = true, failed = true } }
local Guard = {}

Guard.__index = Guard

local function fail(message)
	error("Gator run limits: " .. redact.text(tostring(message)), 3)
end

local function integer(value, name)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail(name .. " must be a non-negative integer")
	end
	return value
end

local function budget(value)
	if type(value) ~= "table" or next(value) == nil then
		fail("budget must be a non-empty object")
	end
	local result = {}
	for key, limit in pairs(value) do
		if key ~= "input_tokens" and key ~= "output_tokens" and key ~= "total_tokens" then
			fail("budget contains unsupported field: " .. tostring(key))
		end
		result[key] = integer(limit, "budget." .. key)
	end
	return result
end

local function usage(value)
	if type(value) ~= "table" then
		fail("usage must be an object")
	end
	local result = {}
	for key, amount in pairs(value) do
		if key ~= "input_tokens" and key ~= "output_tokens" and key ~= "total_tokens" then
			fail("usage contains unsupported field: " .. tostring(key))
		end
		result[key] = integer(amount, "usage." .. key)
	end
	return result
end

local function status(self)
	return vim.deepcopy({ run_id = self.run.id, state = self.state, kind = self.kind, reason = self.reason })
end

local function cancel(self, kind, reason)
	if self.state ~= "ready" then
		return status(self)
	end
	local decision = { run_id = self.run.id, kind = kind, reason = redact.text(reason) }
	local ok, value = pcall(self.cancel_callback, vim.deepcopy(decision))
	if not ok or value ~= true then
		self.state = "failed"
		self.kind = kind
		self.reason = redact.text(ok and "cancellation hook rejected policy action" or tostring(value))
		return status(self)
	end
	self.state = "cancelled"
	self.kind = kind
	self.reason = decision.reason
	return status(self)
end

function M.new(opts)
	if type(opts) ~= "table" or not run.is(opts.run) then
		fail("new requires a persistent Gator run")
	end
	for key in pairs(opts) do
		if key ~= "run" and key ~= "timeout_ms" and key ~= "budget" and key ~= "cancel" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	if opts.timeout_ms == nil and opts.budget == nil then
		fail("new requires a timeout_ms or budget policy")
	end
	if
		opts.timeout_ms ~= nil
		and (type(opts.timeout_ms) ~= "number" or opts.timeout_ms < 1 or opts.timeout_ms % 1 ~= 0)
	then
		fail("timeout_ms must be a positive integer")
	end
	if type(opts.cancel) ~= "function" then
		fail("new requires a cancellation hook")
	end
	return setmetatable({
		run = run.from_record(run.to_record(opts.run)),
		timeout_ms = opts.timeout_ms,
		budget = opts.budget and budget(opts.budget) or nil,
		cancel_callback = opts.cancel,
		state = "ready",
	}, Guard)
end

function M.is(value)
	return getmetatable(value) == Guard
end

function Guard:status()
	if not M.is(self) then
		fail("status requires a run-limit guard")
	end
	return status(self)
end

function Guard:observe(value)
	if not M.is(self) then
		fail("observe requires a run-limit guard")
	end
	if type(value) ~= "table" then
		fail("observe requires values")
	end
	for key in pairs(value) do
		if key ~= "elapsed_ms" and key ~= "usage" then
			fail("observe contains unsupported field: " .. tostring(key))
		end
	end
	if self.state ~= "ready" then
		return self:status()
	end
	if self.timeout_ms ~= nil then
		if value.elapsed_ms == nil then
			return { run_id = self.run.id, state = "unavailable", reason = "elapsed runtime is unavailable" }
		end
		integer(value.elapsed_ms, "elapsed_ms")
		if value.elapsed_ms >= self.timeout_ms then
			return cancel(self, "timeout", "run timeout policy exceeded")
		end
	end
	if self.budget ~= nil then
		if value.usage == nil then
			return { run_id = self.run.id, state = "unavailable", reason = "provider usage is unavailable" }
		end
		local observed = usage(value.usage)
		for key, limit in pairs(self.budget) do
			if observed[key] == nil then
				return { run_id = self.run.id, state = "unavailable", reason = "provider " .. key .. " is unavailable" }
			end
			if observed[key] >= limit then
				return cancel(self, "budget", "run " .. key .. " budget exceeded")
			end
		end
	end
	return self:status()
end

function Guard:cancel(reason)
	if not M.is(self) then
		fail("cancel requires a run-limit guard")
	end
	if reason ~= nil and (type(reason) ~= "string" or reason == "") then
		fail("cancellation reason must be non-empty text")
	end
	return cancel(self, "cancelled", reason or "run cancellation requested")
end

return M
