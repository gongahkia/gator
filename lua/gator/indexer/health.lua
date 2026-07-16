local M = {}

local function fail(message)
	error("Gator indexer health: " .. message, 3)
end

function M.check(opts)
	if type(opts) ~= "table" or type(opts.lifecycle) ~= "table" or type(opts.lifecycle.status) ~= "function" then
		fail("check requires a lifecycle")
	end
	local status = opts.lifecycle:status()
	if status and status.state == "running" then
		return { available = true, mode = "indexer" }
	end
	return {
		available = false,
		mode = "lexical_manual",
		reason = status and ("indexer is " .. status.state) or "indexer is not running",
		repair = { action = "restart-indexer" },
	}
end

function M.repair(opts)
	if type(opts) ~= "table" or type(opts.lifecycle) ~= "table" or type(opts.lifecycle.recover) ~= "function" then
		fail("repair requires a recoverable lifecycle")
	end
	return opts.lifecycle:recover()
end

return M
