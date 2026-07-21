local redact = require("gator.policy.redact")
local run = require("gator.core.run")

local M = {
	api_version = 1,
	states = {
		attached = true,
		detached = true,
		orphaned = true,
		terminal = true,
		unavailable = true,
		failed = true,
		cancelled = true,
	},
}
local terminal = { completed = true, failed = true, cancelled = true }

local function fail(message)
	error("Gator process classification: " .. redact.text(tostring(message)), 3)
end

local function result(value, status, reason)
	return {
		run_id = value.id,
		provider = value.provider.name,
		pid = value.process.pid,
		status = status,
		reason = reason and redact.text(reason) or nil,
	}
end

local function session_reference(value)
	if not value.provider.session_id then
		return nil
	end
	return { provider = value.provider.name, id = value.provider.session_id, owner = "provider" }
end

local function session_available(value)
	if type(value) == "boolean" then
		return value, nil
	end
	if type(value) ~= "table" then
		return nil, "provider session probe returned unsupported data"
	end
	for key in pairs(value) do
		if key ~= "available" and key ~= "reason" then
			return nil, "provider session probe returned unsupported field: " .. tostring(key)
		end
	end
	if type(value.available) ~= "boolean" then
		return nil, "provider session probe must return availability"
	end
	if value.reason ~= nil and (type(value.reason) ~= "string" or value.reason == "") then
		return nil, "provider session probe reason must be non-empty text"
	end
	return value.available, value.reason
end

function M.classify(opts)
	if type(opts) ~= "table" then
		fail("classify requires options")
	end
	for key in pairs(opts) do
		if key ~= "run" and key ~= "alive" and key ~= "session_probe" then
			fail("classify contains unsupported field: " .. tostring(key))
		end
	end
	if not run.is(opts.run) then
		fail("classify requires a persistent Gator run")
	end
	if opts.alive ~= nil and type(opts.alive) ~= "function" then
		fail("alive must be a function")
	end
	if opts.session_probe ~= nil and type(opts.session_probe) ~= "function" then
		fail("session_probe must be a function")
	end
	local value = run.from_record(run.to_record(opts.run))
	if value.state == "cancelled" then
		return result(value, "cancelled")
	end
	if terminal[value.state] then
		return result(value, "terminal")
	end
	if value.state == "queued" then
		return result(value, "unavailable", "process has not started")
	end
	if not opts.alive then
		return result(value, "unavailable", "process liveness probe is unavailable")
	end
	local ok, live = pcall(opts.alive, value.process.pid)
	if not ok or type(live) ~= "boolean" then
		return result(value, "failed", ok and "process liveness probe must return a boolean" or tostring(live))
	end
	if not live then
		return result(value, "detached", "managed process is not live")
	end
	local reference = session_reference(value)
	if not reference then
		return result(value, "orphaned", "provider session reference is unavailable")
	end
	if not opts.session_probe then
		return result(value, "unavailable", "provider session probe is unavailable")
	end
	local probed, state = pcall(opts.session_probe, vim.deepcopy(reference))
	if not probed then
		return result(value, "failed", tostring(state))
	end
	local available, reason = session_available(state)
	if available == nil then
		return result(value, "failed", reason)
	end
	if not available then
		return result(value, "orphaned", reason or "provider session is unavailable")
	end
	local classified = result(value, "attached")
	classified.session = reference
	return classified
end

return M
