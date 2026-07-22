local provider_event = require("gator.core.provider_event")
local redact = require("gator.policy.redact")
local task = require("gator.core.task")
local M = { schema_version = 1 }
local Evidence = {}
local decisions = { ["message.thought"] = true, ["permission.decision"] = true, ["tool.call"] = true }
local outcomes = {
	["message.completed"] = true,
	["tool.result"] = true,
	["file.change"] = true,
	["file.diff"] = true,
	["run.completed"] = true,
	["run.error"] = true,
}

Evidence.__index = Evidence

local function fail(message)
	error("Gator handoff evidence: " .. redact.text(tostring(message)), 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function timestamp(value, name)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail(name .. " must be a non-negative integer timestamp")
	end
	return value
end

local function source(value)
	if type(value) ~= "table" then
		fail("source must be a table")
	end
	for key in pairs(value) do
		if key ~= "provider" and key ~= "run_id" and key ~= "session" then
			fail("source contains unsupported field: " .. tostring(key))
		end
	end
	local result =
		{ provider = identifier(value.provider, "source.provider"), run_id = identifier(value.run_id, "source.run_id") }
	if value.session ~= nil then
		if type(value.session) ~= "table" then
			fail("source.session must be a provider-owned reference")
		end
		for key in pairs(value.session) do
			if key ~= "provider" and key ~= "id" and key ~= "owner" then
				fail("source.session contains unsupported field: " .. tostring(key))
			end
		end
		if value.session.provider ~= result.provider or value.session.owner ~= "provider" then
			fail("source.session must remain provider-owned")
		end
		if type(value.session.id) ~= "string" or value.session.id == "" then
			fail("source.session.id must be a non-empty opaque identifier")
		end
		result.session = { provider = result.provider, id = value.session.id, owner = "provider" }
	end
	return result
end

local function summary(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be non-empty text")
	end
	return redact.text(value)
end

local function evidence(value, allowed, name)
	if type(value) ~= "table" then
		fail(name .. " must be an ordered array")
	end
	if not vim.islist(value) then
		fail(name .. " must be an ordered array")
	end
	local result, seen = {}, {}
	for index, item in ipairs(value) do
		if type(item) ~= "table" then
			fail(name .. "[" .. index .. "] must be a table")
		end
		for key in pairs(item) do
			if key ~= "event_id" and key ~= "type" and key ~= "at" and key ~= "summary" then
				fail(name .. "[" .. index .. "] contains unsupported field: " .. tostring(key))
			end
		end
		local event_id = identifier(item.event_id, name .. " event id")
		if seen[event_id] then
			fail(name .. " must not duplicate event ids")
		end
		if type(item.type) ~= "string" or not allowed[item.type] then
			fail(name .. " event type is unsupported")
		end
		seen[event_id] = true
		result[index] = {
			event_id = event_id,
			type = item.type,
			at = timestamp(item.at, name .. " event timestamp"),
			summary = summary(item.summary, name .. " event summary"),
		}
	end
	return result
end

local function attrs(value)
	if type(value) ~= "table" then
		fail("attributes must be a table")
	end
	for key in pairs(value) do
		if
			key ~= "schema_version"
			and key ~= "task_id"
			and key ~= "state"
			and key ~= "reason"
			and key ~= "source"
			and key ~= "decisions"
			and key ~= "outcomes"
		then
			fail("attributes contain unsupported field: " .. tostring(key))
		end
	end
	if value.schema_version ~= nil and value.schema_version ~= M.schema_version then
		fail("schema_version is unsupported")
	end
	local result = {
		task_id = identifier(value.task_id, "task_id"),
		state = value.state,
		decisions = evidence(value.decisions or {}, decisions, "decisions"),
		outcomes = evidence(value.outcomes or {}, outcomes, "outcomes"),
	}
	local event_ids = {}
	for _, group in ipairs({ result.decisions, result.outcomes }) do
		for _, item in ipairs(group) do
			if event_ids[item.event_id] then
				fail("evidence must not duplicate event ids")
			end
			event_ids[item.event_id] = true
		end
	end
	if result.state == "ready" then
		if value.reason ~= nil or #result.decisions + #result.outcomes == 0 then
			fail("ready evidence requires source events and no reason")
		end
		result.source = source(value.source)
	elseif result.state == "unavailable" then
		if value.source ~= nil or #result.decisions + #result.outcomes ~= 0 then
			fail("unavailable evidence must not retain source events")
		end
		result.reason = summary(value.reason, "reason")
	else
		fail("state must be ready or unavailable")
	end
	return result
end

local function event_summary(value)
	local payload = value.payload
	if value.type == "message.thought" then
		return payload.summary or payload.text or "source reasoning"
	end
	if value.type == "permission.decision" then
		return "permission " .. tostring(payload.request_id) .. " was " .. tostring(payload.decision)
	end
	if value.type == "tool.call" then
		return "requested tool " .. tostring(payload.name)
	end
	if value.type == "message.completed" then
		return payload.text or payload.summary or "source message completed"
	end
	if value.type == "tool.result" then
		return "tool " .. tostring(payload.call_id) .. " " .. tostring(payload.state)
	end
	if value.type == "file.change" then
		return tostring(payload.kind) .. " " .. tostring(payload.path)
	end
	if value.type == "file.diff" then
		return "changed " .. tostring(#payload.paths) .. " file(s)"
	end
	if value.type == "run.error" then
		return tostring(payload.kind or "unknown")
			.. (payload.retryable and " retryable" or "")
			.. " error: "
			.. (payload.message or "source run failed")
	end
	return "source run completed"
end

local function record(value)
	return {
		event_id = value.id,
		type = value.type,
		at = value.at,
		summary = event_summary(value),
	}
end

local function event_source(value)
	local result = { provider = value.provider.name, run_id = value.run_id }
	if value.provider.session_id then
		result.session = { provider = value.provider.name, id = value.provider.session_id, owner = "provider" }
	end
	return result
end

local function same_source(left, right)
	return left.provider == right.provider
		and left.run_id == right.run_id
		and (left.session and left.session.id or nil) == (right.session and right.session.id or nil)
end

function M.new(value)
	return setmetatable(attrs(value), Evidence)
end

function M.is(value)
	return getmetatable(value) == Evidence
end

function M.to_record(value)
	if not M.is(value) then
		fail("value must be created by gator.context.handoff_evidence.new")
	end
	local canonical = attrs(value)
	return {
		schema_version = M.schema_version,
		task_id = canonical.task_id,
		state = canonical.state,
		reason = canonical.reason,
		source = canonical.source,
		decisions = canonical.decisions,
		outcomes = canonical.outcomes,
	}
end

function M.from_record(value)
	return M.new(value)
end

function M.capture(opts)
	if type(opts) ~= "table" then
		fail("capture requires options")
	end
	for key in pairs(opts) do
		if key ~= "task" and key ~= "events" then
			fail("capture options contain unsupported field: " .. tostring(key))
		end
	end
	if not task.is(opts.task) then
		fail("capture requires a task created by gator.core.task.new")
	end
	if type(opts.events) ~= "table" or not vim.islist(opts.events) then
		fail("events must be an ordered array")
	end
	local values = {}
	for index, value in ipairs(opts.events) do
		if not provider_event.is(value) then
			fail("events[" .. index .. "] must be a normalized provider event")
		end
		values[index] = provider_event.from_record(provider_event.to_record(value))
	end
	table.sort(values, function(left, right)
		return left.sequence == right.sequence and left.id < right.id or left.sequence < right.sequence
	end)
	local source, captured, decision_records, outcome_records = nil, {}, {}, {}
	for _, value in ipairs(values) do
		local group = decisions[value.type] and decision_records or outcomes[value.type] and outcome_records or nil
		if group then
			local next_source = event_source(value)
			if source and not same_source(source, next_source) then
				fail("source events must share a provider-native run and session")
			end
			if captured[value.id] then
				fail("source events must not duplicate ids")
			end
			source, captured[value.id] = next_source, true
			table.insert(group, record(value))
		end
	end
	if not source then
		return M.new({
			task_id = opts.task.id,
			state = "unavailable",
			reason = "source run emitted no handoff decision or outcome evidence",
		})
	end
	return M.new({
		task_id = opts.task.id,
		state = "ready",
		source = source,
		decisions = decision_records,
		outcomes = outcome_records,
	})
end

return M
