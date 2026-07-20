local runs = require("gator.core.run")
local errors = require("gator.error")
local M = {}
local terminal = { completed = true, failed = true, cancelled = true }

local function fail(message)
	error("Gator workspace recovery: " .. message, 3)
end

local function reference(run)
	return {
		run_id = run.id,
		task_id = run.task_id,
		provider = run.provider.name,
		session_id = run.provider.session_id,
		workspace_root = run.workspace.root,
	}
end

local function probe(callback, run)
	local ok, value = pcall(callback, reference(run))
	if not ok or type(value) ~= "table" then
		errors.raise(errors.recovery("probe_failed", { detail = "run " .. run.id .. " liveness probe failed" }))
	end
	for key in pairs(value) do
		if key ~= "live" and key ~= "resumable" then
			errors.raise(
				errors.recovery(
					"probe_failed",
					{ detail = "run liveness probe returned unsupported field: " .. tostring(key) }
				)
			)
		end
	end
	if type(value.live) ~= "boolean" or type(value.resumable) ~= "boolean" then
		errors.raise(
			errors.recovery("probe_failed", { detail = "run liveness probe must return live and resumable booleans" })
		)
	end
	return value
end

function M.recover(opts)
	if type(opts) ~= "table" then
		fail("recover requires options")
	end
	for key in pairs(opts) do
		if key ~= "runs" and key ~= "probe" and key ~= "reconnect" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	if type(opts.runs) ~= "table" or not vim.islist(opts.runs) or type(opts.probe) ~= "function" then
		fail("recover requires runs and a probe")
	end
	if opts.reconnect ~= nil and type(opts.reconnect) ~= "function" then
		fail("reconnect must be a function")
	end
	local result, ids = {}, {}
	for index, run in ipairs(opts.runs) do
		if not runs.is(run) then
			fail("run " .. index .. " must be a Gator run")
		end
		if ids[run.id] then
			fail("run id is duplicated: " .. run.id)
		end
		ids[run.id] = true
		local value = { run_id = run.id, provider = run.provider.name, session_id = run.provider.session_id }
		if terminal[run.state] then
			value.status = "terminal"
		elseif run.state == "queued" then
			value.status = "queued"
		else
			local state = probe(opts.probe, run)
			if not state.live then
				value.status = "interrupted"
				value.reason = "agent process is not live"
			elseif not run.provider.session_id then
				value.status = "orphaned"
				value.reason = "provider session is unavailable"
			elseif not state.resumable then
				value.status = "orphaned"
				value.reason = "provider session cannot resume"
			elseif not opts.reconnect then
				value.status = "orphaned"
				value.reason = "provider reconnect is unavailable"
			else
				local ok, reconnected = pcall(opts.reconnect, reference(run))
				if ok and reconnected == true then
					value.status = "reconnected"
				else
					value.status = "orphaned"
					value.reason = "provider reconnect failed"
				end
			end
		end
		table.insert(result, value)
	end
	return result
end

return M
