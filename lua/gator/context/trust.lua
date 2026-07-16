local pack = require("gator.context.pack")
local M = {}
local modes = { provenance = true, repository = true, manual = true }

local function fail(message)
	error("Gator context trust: " .. message, 3)
end

local function mode(value)
	if type(value) ~= "string" or not modes[value] then
		fail("mode must be provenance, repository, or manual")
	end
	return value
end

function M.decide(value, entry)
	value = mode(value)
	if type(entry) ~= "table" or type(entry.trust) ~= "string" then
		fail("entry must declare a trust level")
	end
	if value == "manual" then
		return { allowed = false, reason = "blocked: strict manual trust requires explicit selection" }
	end
	if entry.trust == "provenance" then
		return { allowed = true, reason = "allowed by provenance trust" }
	end
	if value == "repository" and entry.trust == "repository" then
		return { allowed = true, reason = "allowed by repository trust" }
	end
	return { allowed = false, reason = "blocked: " .. value .. " trust does not allow " .. entry.trust .. " context" }
end

function M.policy(value)
	value = mode(value)
	return function(entry)
		return M.decide(value, entry)
	end
end

function M.evaluate(opts)
	if type(opts) ~= "table" or not pack.is(opts.pack) then
		fail("evaluate requires a context pack")
	end
	for key in pairs(opts) do
		if key ~= "pack" and key ~= "mode" then
			fail("evaluate contains unsupported field: " .. tostring(key))
		end
	end
	local value = mode(opts.mode)
	local entries, decisions = {}, {}
	for index, entry in ipairs(opts.pack.entries) do
		local decision = M.decide(value, entry)
		entries[index] = vim.deepcopy(entry)
		entries[index].policy_decision = decision.reason
		decisions[index] = { id = entry.id, allowed = decision.allowed, reason = decision.reason }
	end
	return pack.new({ id = opts.pack.id, task_id = opts.pack.task_id, entries = entries }), decisions
end

return M
