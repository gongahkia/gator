local capabilities = require("gator.adapters.capabilities")
local estimate = require("gator.context.estimate")
local pack = require("gator.context.handoff_pack")
local redact = require("gator.policy.redact")
local M = {}

local function fail(message)
	error("Gator handoff cost: " .. redact.text(tostring(message)), 3)
end

local function cached(value)
	return math.ceil(value * 1.1)
end

local function unavailable(provider, value, reason, entries)
	return {
		status = "unavailable",
		provider = provider,
		pack_id = value.id,
		task_id = value.task_id,
		reason = redact.text(reason),
		entries = entries or {},
	}
end

function M.estimate(opts)
	if type(opts) ~= "table" then
		fail("estimate requires options")
	end
	for key in pairs(opts) do
		if key ~= "pack" and key ~= "capabilities" and key ~= "count" then
			fail("estimate contains unsupported field: " .. tostring(key))
		end
	end
	if not pack.is(opts.pack) or not capabilities.is(opts.capabilities) then
		fail("estimate requires a canonical handoff pack and capability contract")
	end
	if opts.count ~= nil and type(opts.count) ~= "function" then
		fail("count must be a function")
	end
	local value = pack.to_record(opts.pack)
	local supported, reason = capabilities.supports(opts.capabilities, "model", "token_count")
	if not supported then
		return unavailable(opts.capabilities.provider, value, "target token counting is unavailable: " .. reason)
	end
	local entries, total = {}, 0
	for index, entry in ipairs(value.entries) do
		local result
		if entry.content then
			result = estimate.tokens({
				capabilities = opts.capabilities,
				text = redact.text(entry.content),
				count = opts.count,
			})
		elseif entry.token_estimate.status == "estimated" then
			result = { status = "estimated", tokens = cached(entry.token_estimate.tokens) }
		else
			result = { status = "unavailable", reason = entry.token_estimate.reason }
		end
		entries[index] = { id = entry.id, status = result.status, tokens = result.tokens, reason = result.reason }
		if result.status ~= "estimated" then
			return unavailable(
				opts.capabilities.provider,
				value,
				"target context cost is unavailable for " .. entry.id .. ": " .. result.reason,
				entries
			)
		end
		total = total + result.tokens
	end
	return {
		status = "estimated",
		provider = opts.capabilities.provider,
		pack_id = value.id,
		task_id = value.task_id,
		tokens = total,
		entries = entries,
	}
end

return M
