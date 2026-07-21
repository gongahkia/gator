local classify = require("gator.core.process_classification")
local redact = require("gator.policy.redact")
local run = require("gator.core.run")

local M = { api_version = 1 }

local function fail(message)
	error("Gator run reconciliation: " .. redact.text(tostring(message)), 3)
end

local function timestamp(value)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail("at must be a non-negative integer timestamp")
	end
	return value
end

local function runs(value)
	if type(value) ~= "table" or not vim.islist(value) then
		fail("runs must be an array")
	end
	local result, ids = {}, {}
	for index, value in ipairs(value) do
		if not run.is(value) then
			fail("run " .. index .. " must be a persistent Gator run")
		end
		if ids[value.id] then
			fail("run id is duplicated: " .. value.id)
		end
		ids[value.id] = true
		result[index] = run.from_record(run.to_record(value))
	end
	return result
end

local function event_id(value)
	return "reconcile-" .. value.id
end

local function reconciled(value, result, at)
	local id = event_id(value)
	for _, event in ipairs(value.events) do
		if event.id == id then
			return value
		end
	end
	return run.append_event(
		value,
		run.event({
			id = id,
			run_id = value.id,
			type = "run.reconciled",
			at = at,
			payload = { status = result.status, reason = result.reason },
		})
	)
end

local function store(value, result, opts)
	local audited = reconciled(value, result, opts.at)
	local ok, persisted = pcall(opts.store.put, opts.store, audited)
	if not ok or not run.is(persisted) then
		return nil, redact.text(ok and "store returned an invalid run" or tostring(persisted))
	end
	return persisted
end

local function reconnect(value, opts)
	if not value.session then
		return { status = "orphaned", reason = "provider session reference is unavailable" }
	end
	if not opts.reconnect then
		return { status = "unavailable", reason = "provider reconnect is unavailable" }
	end
	local ok, connected = pcall(opts.reconnect, vim.deepcopy(value.session))
	if not ok then
		return { status = "failed", reason = redact.text(tostring(connected)) }
	end
	if connected ~= true then
		return { status = "orphaned", reason = "provider reconnect rejected the session" }
	end
	return { status = "reconnected" }
end

local function outcome(value, opts)
	local classified = classify.classify({
		run = value,
		alive = opts.alive,
		session_probe = opts.session_probe,
	})
	if classified.status == "attached" then
		return reconnect(classified, opts), classified
	end
	return { status = classified.status, reason = classified.reason }, classified
end

function M.reconcile(opts)
	if type(opts) ~= "table" then
		fail("reconcile requires options")
	end
	for key in pairs(opts) do
		if
			key ~= "runs"
			and key ~= "store"
			and key ~= "alive"
			and key ~= "session_probe"
			and key ~= "reconnect"
			and key ~= "at"
		then
			fail("reconcile contains unsupported field: " .. tostring(key))
		end
	end
	if type(opts.store) ~= "table" or type(opts.store.put) ~= "function" then
		fail("reconcile requires a persistent run store")
	end
	if opts.alive ~= nil and type(opts.alive) ~= "function" then
		fail("alive must be a function")
	end
	if opts.session_probe ~= nil and type(opts.session_probe) ~= "function" then
		fail("session_probe must be a function")
	end
	if opts.reconnect ~= nil and type(opts.reconnect) ~= "function" then
		fail("reconnect must be a function")
	end
	local values = runs(opts.runs)
	local configured = vim.deepcopy(opts)
	configured.at = timestamp(opts.at or os.time())
	local result = {}
	for index, value in ipairs(values) do
		local next, classified = outcome(value, configured)
		local persisted, reason = store(value, next, configured)
		if not persisted then
			next = { status = "failed", reason = reason }
		end
		result[index] = {
			run_id = value.id,
			status = next.status,
			reason = next.reason,
			classification = classified.status,
			run = persisted,
		}
	end
	return result
end

return M
