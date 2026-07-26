local errors = require("gator.error")
local redact = require("gator.policy.redact")
local M = {}
local Task = {}

Task.__index = Task

M.lifecycle = {
	draft = true,
	planned = true,
	running = true,
	awaiting_review = true,
	merged = true,
	discarded = true,
	failed = true,
}

local function fail(detail)
	errors.raise(errors.new("task.invalid", "Task entity is invalid", {
		detail = detail,
		remedy = "Correct the task fields before persisting or running it.",
	}))
end

local function require_string(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty string")
	end
	return value
end

local function require_timestamp(value, name)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail(name .. " must be a non-negative integer timestamp")
	end
	return value
end

local function require_list(value, name)
	if type(value) ~= "table" or not vim.islist(value) then
		fail(name .. " must be an array")
	end
	return value
end

local function validate_fields(value, allowed, name)
	for key in pairs(value) do
		if not allowed[key] then
			fail(name .. " contains unsupported field: " .. tostring(key))
		end
	end
end

local function workspace(value)
	if value == nil then
		return nil
	end
	if type(value) ~= "table" then
		fail("workspace must be a table")
	end
	validate_fields(value, { kind = true, root = true }, "workspace")
	if value.kind ~= "project" and value.kind ~= "worktree" then
		fail("workspace.kind must be project or worktree")
	end
	return { kind = value.kind, root = require_string(value.root, "workspace.root") }
end

local function sessions(value)
	local result = {}

	for index, session in ipairs(require_list(value, "sessions")) do
		if type(session) ~= "table" then
			fail("session " .. index .. " must be a table")
		end
		validate_fields(session, { provider = true, id = true, owner = true, mode = true }, "session " .. index)
		if session.owner ~= "provider" and session.owner ~= "gator" then
			fail("session " .. index .. " owner is unsupported")
		end
		local mode = session.mode or "terminal"
		if
			mode ~= "terminal"
			and mode ~= "acp"
			and mode ~= "stream"
			and mode ~= "json"
			and mode ~= "history"
		then
			fail("session " .. index .. " mode is unsupported")
		end
		if session.owner == "gator" and mode ~= "history" then
			fail("session " .. index .. " gator ownership requires history mode")
		end
		result[index] = {
			provider = require_string(session.provider, "session " .. index .. " provider"),
			id = require_string(session.id, "session " .. index .. " id"),
			owner = session.owner,
			mode = mode,
		}
	end
	return result
end

local function evidence(value)
	local result = {}

	for index, reference in ipairs(require_list(value, "evidence")) do
		if type(reference) ~= "table" then
			fail("evidence " .. index .. " must be a table")
		end
		validate_fields(reference, { kind = true, ref = true }, "evidence " .. index)
		result[index] = {
			kind = require_string(reference.kind, "evidence " .. index .. " kind"),
			ref = redact.text(require_string(reference.ref, "evidence " .. index .. " ref")),
		}
	end
	return result
end

function M.new(attrs)
	if type(attrs) ~= "table" then
		fail("attributes must be a table")
	end
	validate_fields(attrs, {
		id = true,
		objective = true,
		lifecycle = true,
		workspace = true,
		sessions = true,
		evidence = true,
		created_at = true,
		updated_at = true,
	}, "task")
	local id = require_string(attrs.id, "id")
	if not id:match("^[a-z][a-z0-9_-]*$") then
		fail("id must be a lowercase identifier")
	end
	local lifecycle = attrs.lifecycle or "draft"
	if not M.lifecycle[lifecycle] then
		fail("lifecycle is unknown: " .. tostring(lifecycle))
	end
	local created_at = attrs.created_at or os.time()
	local updated_at = attrs.updated_at or created_at
	require_timestamp(created_at, "created_at")
	require_timestamp(updated_at, "updated_at")
	if updated_at < created_at then
		fail("updated_at cannot precede created_at")
	end

	return setmetatable({
		id = id,
		objective = redact.text(require_string(attrs.objective, "objective")),
		lifecycle = lifecycle,
		workspace = workspace(attrs.workspace),
		sessions = sessions(attrs.sessions or {}),
		evidence = evidence(attrs.evidence or {}),
		created_at = created_at,
		updated_at = updated_at,
	}, Task)
end

function M.is(value)
	return getmetatable(value) == Task
end

function M.to_record(value)
	if not M.is(value) then
		fail("value must be created by gator.core.task.new")
	end
	local task = M.new(value)
	local record = {
		id = task.id,
		objective = task.objective,
		lifecycle = task.lifecycle,
		sessions = sessions(task.sessions),
		evidence = evidence(task.evidence),
		created_at = task.created_at,
		updated_at = task.updated_at,
	}

	if task.workspace then
		record.workspace = workspace(task.workspace)
	end
	return record
end

function M.from_record(record)
	return M.new(record)
end

return M
