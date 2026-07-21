local provider_event = require("gator.core.provider_event")
local redact = require("gator.policy.redact")

local M = {}

local function fail(message)
	error("Gator approval event: " .. redact.text(tostring(message)), 3)
end

local function text(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be non-empty text")
	end
	return value
end

local function event(value, event_type, allowed)
	if type(value) ~= "table" then
		fail("attributes must be a table")
	end
	for key in pairs(value) do
		if not allowed[key] then
			fail("attributes contain unsupported field: " .. tostring(key))
		end
	end
	return {
		schema_version = provider_event.schema_version,
		id = value.id,
		run_id = value.run_id,
		provider = value.provider,
		sequence = value.sequence,
		at = value.at,
		type = event_type,
	}
end

function M.request(value)
	local result = event(value, "permission.request", {
		id = true,
		run_id = true,
		provider = true,
		sequence = true,
		at = true,
		request_id = true,
		action = true,
		details = true,
	})
	result.payload = {
		request_id = text(value.request_id, "request id"),
		action = text(value.action, "action"),
		details = value.details or {},
	}
	return provider_event.new(result)
end

function M.decision(value)
	local result = event(value, "permission.decision", {
		id = true,
		run_id = true,
		provider = true,
		sequence = true,
		at = true,
		request_id = true,
		decision = true,
		reason = true,
	})
	if value.decision ~= "approved" and value.decision ~= "denied" and value.decision ~= "cancelled" then
		fail("approval decision is unavailable")
	end
	result.payload = { request_id = text(value.request_id, "request id"), decision = value.decision }
	if value.reason ~= nil then
		result.payload.reason = text(value.reason, "reason")
	end
	return provider_event.new(result)
end

return M
