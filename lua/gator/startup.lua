local deferred = { "gator.adapters", "gator.indexer", "gator.github" }
local redact = require("gator.policy.redact")
local runs = require("gator.core.run")
local state_store = require("gator.state")
local M = { name = "startup", api_version = 1, deferred_modules = vim.deepcopy(deferred) }
local terminal = { completed = true, failed = true, cancelled = true }

local function fail(message)
	error("Gator startup: " .. message, 3)
end

local function clock(value)
	if type(value) ~= "number" or value < 0 then
		fail("clock must return a non-negative number")
	end
	return value
end

local function recovery_failure(state, detail)
	detail = redact.text(tostring(detail))
	if detail == "" then
		detail = "local recovery data is unavailable"
	end
	detail = "startup recovery failed: " .. detail
	state:update({ workspace = { status = "failed", detail = detail } })
	return { status = "failed", detail = detail, runs = {} }
end

local function recovery_runs(load)
	local value = load()
	if type(value) ~= "table" or not vim.islist(value) then
		fail("startup recovery loader must return a run array")
	end
	local result, ids = {}, {}
	for index, run in ipairs(value) do
		if not runs.is(run) then
			fail("startup recovery run " .. index .. " must be a Gator run")
		end
		if ids[run.id] then
			fail("startup recovery run id is duplicated: " .. run.id)
		end
		ids[run.id] = true
		local entry = {
			run_id = run.id,
			task_id = run.task_id,
			provider = run.provider.name,
			state = run.state,
			status = terminal[run.state] and "terminal" or "recovery_pending",
		}
		if run.provider.session_id then
			entry.session_id = run.provider.session_id
		end
		table.insert(result, entry)
	end
	return result
end

function M.recover(opts)
	if type(opts) ~= "table" then
		fail("recover requires options")
	end
	for key in pairs(opts) do
		if key ~= "state" and key ~= "load" then
			fail("recover contains unsupported field: " .. tostring(key))
		end
	end
	if not state_store.is(opts.state) then
		fail("recover requires initialized Gator state")
	end
	if type(opts.load) ~= "function" then
		fail("recover requires a local run loader")
	end
	local ok, value = xpcall(function()
		return recovery_runs(opts.load)
	end, debug.traceback)
	if not ok then
		return recovery_failure(opts.state, value)
	end
	local pending = 0
	for _, run in ipairs(value) do
		if run.status == "recovery_pending" then
			pending = pending + 1
		end
	end
	local detail
	local status = "ready"
	if pending > 0 then
		status = "recovering"
		detail = pending .. " nonterminal provider run(s) await explicit recovery"
	end
	opts.state:update({ workspace = { status = status, detail = detail } })
	return { status = status, detail = detail, runs = vim.deepcopy(value) }
end

function M.measure(opts)
	if type(opts) ~= "table" or type(opts.load) ~= "function" then
		fail("measure requires a load function")
	end
	for key in pairs(opts) do
		if key ~= "load" and key ~= "clock" then
			fail("measure contains unsupported field: " .. tostring(key))
		end
	end
	if opts.clock ~= nil and type(opts.clock) ~= "function" then
		fail("measure clock must be a function")
	end
	local before = {}
	for _, module in ipairs(deferred) do
		before[module] = package.loaded[module] ~= nil
	end
	local now = opts.clock or vim.uv.hrtime
	local original, launches = vim.system, 0
	vim.system = function()
		launches = launches + 1
		error("startup attempted a process launch", 0)
	end
	local started = clock(now())
	local ok, value = xpcall(opts.load, debug.traceback)
	local elapsed = clock(now()) - started
	vim.system = original
	if not ok then
		fail("startup loader failed: " .. value)
	end
	if launches > 0 then
		fail("startup launched " .. launches .. " process(es)")
	end
	local loaded = {}
	for _, module in ipairs(deferred) do
		if not before[module] and package.loaded[module] ~= nil then
			table.insert(loaded, module)
		end
	end
	if #loaded > 0 then
		fail("startup loaded deferred module(s): " .. table.concat(loaded, ", "))
	end
	return { duration_ms = elapsed / 1000000, process_launches = launches, deferred_modules = vim.deepcopy(deferred) }
end

return M
