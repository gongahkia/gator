local M = { api_version = 1, schema_version = 2 }
local names = {
	"startup",
	"context",
	"stream",
	"ui_loop",
	"worktree",
	"diff",
	"indexer",
	"timeline",
	"storage",
	"handoff",
}

local function fail(message)
	error("Gator performance suite: " .. message, 3)
end

local function number(value, name)
	if type(value) ~= "number" or value < 0 then
		fail(name .. " must return a non-negative number")
	end
	return value
end

local function integer(value, name)
	if type(value) ~= "number" or value < 1 or value % 1 ~= 0 then
		fail(name .. " must be a positive integer")
	end
	return value
end

local function median(values)
	local sorted = vim.deepcopy(values)
	table.sort(sorted)
	local middle = #sorted / 2
	if middle % 1 == 0 then
		return (sorted[middle] + sorted[middle + 1]) / 2
	end
	return sorted[math.ceil(middle)]
end

local function budget(value, name, samples)
	if value == nil then
		return nil
	end
	if type(value) ~= "table" then
		fail("budgets." .. name .. " must be a table")
	end
	for key in pairs(value) do
		if
			key ~= "duration_ms"
			and key ~= "memory_kb_delta"
			and key ~= "variance_percent"
			and key ~= "sustained_samples"
		then
			fail("budgets." .. name .. " contains unsupported field: " .. tostring(key))
		end
	end
	if type(value.duration_ms) ~= "number" or value.duration_ms < 0 then
		fail("budgets." .. name .. ".duration_ms must be a non-negative number")
	end
	if type(value.memory_kb_delta) ~= "number" or value.memory_kb_delta < 0 then
		fail("budgets." .. name .. ".memory_kb_delta must be a non-negative number")
	end
	if type(value.variance_percent) ~= "number" or value.variance_percent < 0 then
		fail("budgets." .. name .. ".variance_percent must be a non-negative number")
	end
	local sustained = value.sustained_samples or samples
	integer(sustained, "budgets." .. name .. ".sustained_samples")
	if sustained > samples then
		fail("budgets." .. name .. ".sustained_samples cannot exceed samples")
	end
	return {
		duration_ms = value.duration_ms,
		memory_kb_delta = value.memory_kb_delta,
		variance_percent = value.variance_percent,
		sustained_samples = sustained,
	}
end

local function regression(values, limit, variance_percent, sustained_samples)
	local maximum, breaches = limit * (1 + variance_percent / 100), 0
	for _, value in ipairs(values) do
		if value > maximum then
			breaches = breaches + 1
		end
	end
	return breaches >= sustained_samples, breaches, maximum
end

function M.run(opts)
	if type(opts) ~= "table" or type(opts.cases) ~= "table" then
		fail("run requires cases")
	end
	for key in pairs(opts) do
		if
			key ~= "cases"
			and key ~= "clock"
			and key ~= "memory"
			and key ~= "path"
			and key ~= "samples"
			and key ~= "budgets"
		then
			fail("run contains unsupported field: " .. tostring(key))
		end
	end
	local clock, memory = opts.clock or vim.uv.hrtime, opts.memory or function()
		return collectgarbage("count")
	end
	if type(clock) ~= "function" or type(memory) ~= "function" then
		fail("clock and memory must be functions")
	end
	local samples = opts.samples or 1
	integer(samples, "samples")
	if opts.budgets ~= nil and type(opts.budgets) ~= "table" then
		fail("budgets must be a table")
	end
	local configured_budgets = {}
	if opts.budgets then
		local known = {}
		for _, name in ipairs(names) do
			known[name] = true
		end
		for name in pairs(opts.budgets) do
			if not known[name] then
				fail("budgets contains unknown workload: " .. tostring(name))
			end
		end
		for _, name in ipairs(names) do
			configured_budgets[name] = budget(opts.budgets[name], name, samples)
			if not configured_budgets[name] then
				fail("budgets must define every workload: " .. name)
			end
		end
	end
	local metrics = {}
	local regressions = {}
	for _, name in ipairs(names) do
		if type(opts.cases[name]) ~= "function" then
			fail("cases." .. name .. " must be a function")
		end
		local durations, memory_deltas = {}, {}
		for _ = 1, samples do
			local started, before = number(clock(), "clock"), number(memory(), "memory")
			local ok = pcall(opts.cases[name])
			if not ok then
				fail(name .. " benchmark failed")
			end
			local elapsed, after = number(clock(), "clock") - started, number(memory(), "memory")
			if elapsed < 0 then
				fail("clock moved backwards")
			end
			table.insert(durations, elapsed / 1000000)
			table.insert(memory_deltas, after - before)
		end
		local value = {
			name = name,
			duration_ms = median(durations),
			memory_kb_delta = median(memory_deltas),
			duration_ms_samples = durations,
			memory_kb_delta_samples = memory_deltas,
		}
		local configured = configured_budgets[name]
		if configured then
			local duration_regression, duration_breaches, duration_maximum =
				regression(durations, configured.duration_ms, configured.variance_percent, configured.sustained_samples)
			local memory_regression, memory_breaches, memory_maximum = regression(
				memory_deltas,
				configured.memory_kb_delta,
				configured.variance_percent,
				configured.sustained_samples
			)
			value.budget = configured
			value.regression = {
				duration = duration_regression,
				memory = memory_regression,
				duration_breaches = duration_breaches,
				memory_breaches = memory_breaches,
				duration_maximum_ms = duration_maximum,
				memory_maximum_kb_delta = memory_maximum,
			}
			if duration_regression or memory_regression then
				table.insert(regressions, name)
			end
		end
		table.insert(metrics, value)
	end
	local report =
		{ schema_version = M.schema_version, samples = samples, metrics = metrics, regressions = regressions }
	if opts.path then
		if type(opts.path) ~= "string" or opts.path == "" then
			fail("path must be a non-empty string")
		end
		local parent = vim.fn.fnamemodify(opts.path, ":h")
		if vim.fn.isdirectory(parent) ~= 1 or vim.fn.writefile({ vim.json.encode(report) }, opts.path) ~= 0 then
			fail("cannot write benchmark artifact")
		end
	end
	if #regressions > 0 then
		fail("sustained actionable regression: " .. table.concat(regressions, ", "))
	end
	return report
end

return M
