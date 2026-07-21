local provider_event = require("gator.core.provider_event")
local redact = require("gator.policy.redact")

local M = {}

local function fail(message)
	error("Gator usage event: " .. redact.text(tostring(message)), 3)
end

local function base(value, event_type, allowed)
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

function M.usage(value)
	local result = base(value, "usage.update", {
		id = true,
		run_id = true,
		provider = true,
		sequence = true,
		at = true,
		input_tokens = true,
		output_tokens = true,
		total_tokens = true,
	})
	for _, name in ipairs({ "input_tokens", "output_tokens", "total_tokens" }) do
		if value[name] ~= nil and (type(value[name]) ~= "number" or value[name] < 0 or value[name] % 1 ~= 0) then
			fail(name .. " must be a non-negative integer")
		end
	end
	result.payload = { input = value.input_tokens, output = value.output_tokens, total = value.total_tokens }
	return provider_event.new(result)
end

function M.compaction(value)
	local result = base(value, "context.compacted", {
		id = true,
		run_id = true,
		provider = true,
		sequence = true,
		at = true,
		before_tokens = true,
		after_tokens = true,
		summary = true,
	})
	for _, name in ipairs({ "before_tokens", "after_tokens" }) do
		if type(value[name]) ~= "number" or value[name] < 0 or value[name] % 1 ~= 0 then
			fail(name .. " must be a non-negative integer")
		end
	end
	if value.after_tokens > value.before_tokens then
		fail("compaction cannot increase context tokens")
	end
	result.payload = { before = value.before_tokens, after = value.after_tokens, summary = value.summary or "" }
	return provider_event.new(result)
end

return M
