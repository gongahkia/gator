local lifecycle = require("gator.core.lifecycle")
local task = require("gator.core.task")
local M = {}
local targets = { accept = "merged", reject = "discarded" }

local function fail(message)
	error("Gator review actions: " .. message, 3)
end

local function reviewer(value)
	if type(value) ~= "string" or value == "" or value:find("\n", 1, true) then
		fail("reviewer must be a non-empty single-line string")
	end
	return value
end

local function timestamp(value, name)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail(name .. " must be a non-negative integer timestamp")
	end
	return value
end

local function receipt(value)
	if type(value) ~= "table" then
		fail("receipt must be a table")
	end
	for key in pairs(value) do
		if
			key ~= "task_id"
			and key ~= "action"
			and key ~= "ref"
			and key ~= "previous"
			and key ~= "next"
			and key ~= "reviewer"
			and key ~= "at"
		then
			fail("receipt contains unsupported field: " .. tostring(key))
		end
	end
	if type(value.task_id) ~= "string" or not value.task_id:match("^[a-z][a-z0-9_-]*$") then
		fail("receipt task_id must be a lowercase identifier")
	end
	if
		type(value.action) ~= "string"
		or not targets[value.action]
		or value.next ~= targets[value.action]
		or value.previous ~= "awaiting_review"
	then
		fail("receipt action is invalid")
	end
	if type(value.ref) ~= "string" or value.ref == "" then
		fail("receipt ref must be a non-empty string")
	end
	return {
		task_id = value.task_id,
		action = value.action,
		ref = value.ref,
		previous = value.previous,
		next = value.next,
		reviewer = reviewer(value.reviewer),
		at = timestamp(value.at, "receipt at"),
	}
end

local function apply(action, opts)
	if type(opts) ~= "table" or not task.is(opts.task) then
		fail(action .. " requires a Gator task")
	end
	for key in pairs(opts) do
		if key ~= "task" and key ~= "reviewer" and key ~= "at" then
			fail(action .. " contains unsupported field: " .. tostring(key))
		end
	end
	if opts.task.lifecycle ~= "awaiting_review" then
		fail(action .. " requires a task awaiting review")
	end
	local at = timestamp(opts.at or os.time(), "at")
	local changed = lifecycle.transition(opts.task, targets[action], at)
	local identity = reviewer(opts.reviewer)
	local ref = "review-action://"
		.. vim.fn.sha256(opts.task.id .. "\0" .. action .. "\0" .. identity .. "\0" .. at):sub(1, 24)
	local record = task.to_record(changed)
	table.insert(record.evidence, { kind = "review-action", ref = ref })
	return {
		task = task.from_record(record),
		receipt = {
			task_id = opts.task.id,
			action = action,
			ref = ref,
			previous = "awaiting_review",
			next = targets[action],
			reviewer = identity,
			at = at,
		},
	}
end

function M.accept(opts)
	return apply("accept", opts)
end

function M.reject(opts)
	return apply("reject", opts)
end

function M.undo(opts)
	if type(opts) ~= "table" or not task.is(opts.task) then
		fail("undo requires a Gator task and receipt")
	end
	for key in pairs(opts) do
		if key ~= "task" and key ~= "receipt" and key ~= "at" then
			fail("undo contains unsupported field: " .. tostring(key))
		end
	end
	local value = receipt(opts.receipt)
	if opts.task.id ~= value.task_id or opts.task.lifecycle ~= value.next then
		fail("receipt does not match the task action")
	end
	local at = timestamp(opts.at or os.time(), "at")
	if at < opts.task.updated_at then
		fail("undo timestamp cannot precede the task update")
	end
	local record = task.to_record(opts.task)
	local retained, removed = {}, false
	for _, evidence in ipairs(record.evidence) do
		if evidence.kind == "review-action" and evidence.ref == value.ref then
			removed = true
		else
			table.insert(retained, evidence)
		end
	end
	if not removed then
		fail("receipt evidence is unavailable")
	end
	record.evidence = retained
	record.lifecycle = value.previous
	record.updated_at = at
	return task.from_record(record)
end

return M
