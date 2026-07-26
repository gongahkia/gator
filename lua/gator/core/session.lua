local errors = require("gator.error")
local task = require("gator.core.task")
local M = {}
local Session = {}

Session.__index = Session

local function fail(detail)
	errors.raise(errors.new("session.invalid", "Session link is invalid", {
		detail = detail,
		remedy = "Link an opaque provider session or Gator-owned local history to its task.",
	}))
end

local function require_string(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty string")
	end
	return value
end

local function validate_fields(value, allowed)
	for key in pairs(value) do
		if not allowed[key] then
			fail("unsupported session field: " .. tostring(key))
		end
	end
end

function M.new(attrs)
	if type(attrs) ~= "table" then
		fail("attributes must be a table")
	end
	validate_fields(attrs, { task_id = true, provider = true, id = true, owner = true, mode = true })
	local task_id = require_string(attrs.task_id, "task_id")
	if not task_id:match("^[a-z][a-z0-9_-]*$") then
		fail("task_id must be a lowercase identifier")
	end
	if attrs.owner ~= "provider" and attrs.owner ~= "gator" then
		fail("owner must be provider or gator")
	end
	local mode = attrs.mode or "terminal"
	if
		mode ~= "terminal"
		and mode ~= "acp"
		and mode ~= "stream"
		and mode ~= "json"
		and mode ~= "history"
	then
		fail("mode is unsupported")
	end
	if attrs.owner == "gator" and mode ~= "history" then
		fail("gator-owned sessions must be local history")
	end

	return setmetatable({
		task_id = task_id,
		provider = require_string(attrs.provider, "provider"),
		id = require_string(attrs.id, "id"),
		owner = attrs.owner,
		mode = mode,
	}, Session)
end

function M.is(value)
	return getmetatable(value) == Session
end

function M.reference(value)
	if not M.is(value) then
		fail("value must be created by gator.core.session.new")
	end
	return { provider = value.provider, id = value.id, owner = value.owner }
end

function M.link(entity, value, updated_at)
	if not task.is(entity) then
		fail("task must be created by gator.core.task.new")
	end
	if not M.is(value) then
		fail("session must be created by gator.core.session.new")
	end
	if entity.id ~= value.task_id then
		fail("session task_id must match the linked task")
	end
	local record = task.to_record(entity)

	for _, reference in ipairs(record.sessions) do
		if reference.provider == value.provider and reference.id == value.id then
			return task.from_record(record)
		end
	end
	updated_at = updated_at or os.time()
	if type(updated_at) ~= "number" or updated_at % 1 ~= 0 or updated_at < entity.updated_at then
		fail("updated_at must be an integer no earlier than the current task timestamp")
	end
	local reference = M.reference(value)
	reference.mode = value.mode
	table.insert(record.sessions, reference)
	record.updated_at = updated_at
	return task.from_record(record)
end

return M
