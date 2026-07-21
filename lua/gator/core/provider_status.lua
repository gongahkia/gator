local provider_event = require("gator.core.provider_event")
local redact = require("gator.policy.redact")

local M = {
	states = { queued = true, running = true, completed = true, failed = true, cancelled = true },
	terminal = { completed = true, failed = true, cancelled = true },
}

local aliases = {
	queued = "queued",
	pending = "queued",
	started = "running",
	running = "running",
	active = "running",
	completed = "completed",
	complete = "completed",
	succeeded = "completed",
	failed = "failed",
	error = "failed",
	cancelled = "cancelled",
	canceled = "cancelled",
}

local transitions = {
	queued = { running = true, failed = true, cancelled = true },
	running = { completed = true, failed = true, cancelled = true },
	completed = {},
	failed = {},
	cancelled = {},
}

local function fail(message)
	error("Gator provider status: " .. redact.text(tostring(message)), 3)
end

function M.normalize(value)
	if type(value) ~= "string" or not aliases[value] then
		fail("provider status is unavailable: " .. tostring(value))
	end
	return aliases[value]
end

function M.transition(previous, next)
	next = M.normalize(next)
	if previous == nil then
		return next
	end
	if type(previous) ~= "string" or not M.states[previous] then
		fail("previous status is unavailable: " .. tostring(previous))
	end
	if previous == next then
		return next
	end
	if not transitions[previous][next] then
		fail("provider status transition is unavailable: " .. previous .. " to " .. next)
	end
	return next
end

function M.from_event(value, previous)
	if not provider_event.is(value) then
		fail("status requires a normalized provider event")
	end
	local domain, action = value.type:match("^([a-z][a-z0-9_-]*)%.([a-z][a-z0-9_-]*)$")
	if domain ~= "run" then
		fail("event does not carry provider run status")
	end
	return M.transition(previous, action)
end

return M
