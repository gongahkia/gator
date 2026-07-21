local provider_event = require("gator.core.provider_event")
local redact = require("gator.policy.redact")

local M = {}
local kinds = {
	unavailable = true,
	authentication = true,
	permission = true,
	rate_limit = true,
	transport = true,
	protocol = true,
	cancelled = true,
	unknown = true,
}

local function fail(message)
	error("Gator error event: " .. redact.text(tostring(message)), 3)
end

function M.new(value)
	if type(value) ~= "table" then
		fail("attributes must be a table")
	end
	for key in pairs(value) do
		if
			key ~= "id"
			and key ~= "run_id"
			and key ~= "provider"
			and key ~= "sequence"
			and key ~= "at"
			and key ~= "kind"
			and key ~= "message"
			and key ~= "retryable"
		then
			fail("attributes contain unsupported field: " .. tostring(key))
		end
	end
	if type(value.kind) ~= "string" or not kinds[value.kind] then
		fail("error kind is unavailable")
	end
	if type(value.message) ~= "string" or value.message == "" or type(value.retryable) ~= "boolean" then
		fail("error message and retryable state are required")
	end
	return provider_event.new({
		schema_version = provider_event.schema_version,
		id = value.id,
		run_id = value.run_id,
		provider = value.provider,
		sequence = value.sequence,
		at = value.at,
		type = "run.error",
		payload = { kind = value.kind, message = value.message, retryable = value.retryable },
	})
end

return M
