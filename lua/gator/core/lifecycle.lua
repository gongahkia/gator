local errors = require("gator.error")
local task = require("gator.core.task")
local M = {}

local transitions = {
	draft = { planned = true, discarded = true },
	planned = { running = true, discarded = true, failed = true },
	running = { awaiting_review = true, discarded = true, failed = true },
	awaiting_review = { running = true, merged = true, discarded = true, failed = true },
	failed = { planned = true, discarded = true },
	merged = {},
	discarded = {},
}

local function fail(detail)
	errors.raise(errors.new("task.transition_invalid", "Task lifecycle transition is invalid", {
		detail = detail,
		remedy = "Use gator.core.lifecycle.can_transition before changing task lifecycle state.",
	}))
end

local function validate_state(state, name)
	if type(state) ~= "string" or not transitions[state] then
		fail(name .. " must be a known task lifecycle state")
	end
	return state
end

function M.can_transition(from, to)
	validate_state(from, "from")
	validate_state(to, "to")
	return transitions[from][to] == true
end

function M.transition(entity, to, updated_at)
	if not task.is(entity) then
		fail("entity must be created by gator.core.task.new")
	end
	local from = validate_state(entity.lifecycle, "task lifecycle")
	validate_state(to, "to")
	if not M.can_transition(from, to) then
		fail("cannot transition from " .. from .. " to " .. to)
	end
	updated_at = updated_at or os.time()
	if type(updated_at) ~= "number" or updated_at % 1 ~= 0 or updated_at < entity.updated_at then
		fail("updated_at must be an integer no earlier than the current task timestamp")
	end

	local record = task.to_record(entity)
	record.lifecycle = to
	record.updated_at = updated_at
	return task.from_record(record)
end

return M
