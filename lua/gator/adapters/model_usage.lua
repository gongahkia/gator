local capabilities = require("gator.adapters.capabilities")
local M = {}

local function fail(message)
	error("Gator model usage: " .. message, 3)
end

local function unavailable(reason)
	return { available = false, reason = reason }
end

local function current(value, now, max_age_ms)
	if
		type(value) ~= "table"
		or type(value.observed_at) ~= "number"
		or value.observed_at < 0
		or value.observed_at % 1 ~= 0
	then
		return nil, "adapter did not prove current data"
	end
	if value.observed_at > now or now - value.observed_at > max_age_ms then
		return nil, "adapter data is stale"
	end
	return value
end

function M.read(opts)
	if type(opts) ~= "table" or not capabilities.is(opts.capabilities) or type(opts.probe) ~= "function" then
		fail("read requires a capability contract and probe")
	end
	if
		type(opts.now) ~= "number"
		or opts.now < 0
		or opts.now % 1 ~= 0
		or type(opts.max_age_ms) ~= "number"
		or opts.max_age_ms < 0
		or opts.max_age_ms % 1 ~= 0
	then
		fail("now and max_age_ms must be non-negative integers")
	end
	local result = {}
	local ok, snapshot = pcall(opts.probe)
	if not ok or type(snapshot) ~= "table" then
		snapshot = {}
	end
	for _, domain in ipairs({ "model", "usage" }) do
		local supported, reason = capabilities.supports(opts.capabilities, domain, "current")
		if not supported then
			result[domain] = unavailable(reason)
		else
			local value, current_reason = current(snapshot[domain], opts.now, opts.max_age_ms)
			if not value then
				result[domain] = unavailable(current_reason)
			elseif domain == "model" and type(value.name) == "string" and value.name ~= "" then
				result.model = { available = true, name = value.name, observed_at = value.observed_at }
			elseif
				domain == "usage"
				and type(value.input_tokens) == "number"
				and type(value.output_tokens) == "number"
				and value.input_tokens >= 0
				and value.output_tokens >= 0
			then
				result.usage = {
					available = true,
					input_tokens = value.input_tokens,
					output_tokens = value.output_tokens,
					observed_at = value.observed_at,
				}
			else
				result[domain] = unavailable("adapter returned invalid current data")
			end
		end
	end
	return result
end

return M
