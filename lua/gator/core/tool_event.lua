local provider_event = require("gator.core.provider_event")
local redact = require("gator.policy.redact")

local M = {}
local states = { completed = true, failed = true, cancelled = true, success = true, error = true, canceled = true }

local function fail(message)
	error("Gator tool event: " .. redact.text(tostring(message)), 3)
end

local function text(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be non-empty text")
	end
	return value
end

local function common(value, allowed)
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
	}
end

function M.call(value)
	local result = common(value, {
		id = true,
		run_id = true,
		provider = true,
		sequence = true,
		at = true,
		call_id = true,
		name = true,
		input = true,
	})
	result.type = "tool.call"
	result.payload =
		{ call_id = text(value.call_id, "call id"), name = text(value.name, "tool name"), input = value.input or {} }
	return provider_event.new(result)
end

function M.result(value)
	local result = common(value, {
		id = true,
		run_id = true,
		provider = true,
		sequence = true,
		at = true,
		call_id = true,
		state = true,
		output = true,
	})
	if type(value.state) ~= "string" or not states[value.state] then
		fail("tool result state is unavailable")
	end
	result.type = "tool.result"
	result.payload = {
		call_id = text(value.call_id, "call id"),
		state = value.state == "success" and "completed"
			or value.state == "error" and "failed"
			or value.state == "canceled" and "cancelled"
			or value.state,
		output = value.output or {},
	}
	return provider_event.new(result)
end

return M
