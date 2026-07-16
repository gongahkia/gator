local M = { api_version = 1, schema_version = 1 }

local function fail(message)
	error("Gator retrieval benchmark: " .. message, 3)
end

local function measurement(value, name)
	if type(value) ~= "number" or value < 0 then
		fail(name .. " must return a non-negative number")
	end
	return value
end

local function queries(value)
	if type(value) ~= "table" or not vim.islist(value) or #value == 0 then
		fail("queries must be a non-empty list")
	end
	for index, query in ipairs(value) do
		if type(query) ~= "string" or query == "" then
			fail("queries[" .. index .. "] must be a non-empty string")
		end
	end
	return value
end

local function benchmark(name, callback, values, now, memory, cancel)
	local result = { samples = 0, cancelled = 0, total_ms = 0, max_ms = 0, memory_kb_delta = 0 }
	for _, query in ipairs(values) do
		local cancelled = function()
			return cancel(name, query) == true
		end
		if cancelled() then
			result.cancelled = result.cancelled + 1
		else
			local started, before = measurement(now(), "clock"), measurement(memory(), "memory")
			local ok = pcall(callback, query, { cancelled = cancelled })
			if not ok then
				fail(name .. " retrieval failed")
			end
			local elapsed = measurement(now(), "clock") - started
			local delta = measurement(memory(), "memory") - before
			if elapsed < 0 then
				fail("clock moved backwards")
			end
			result.samples = result.samples + 1
			result.total_ms = result.total_ms + elapsed / 1000000
			result.max_ms = math.max(result.max_ms, elapsed / 1000000)
			result.memory_kb_delta = math.max(result.memory_kb_delta, delta)
		end
	end
	return result
end

function M.run(opts)
	if type(opts) ~= "table" then
		fail("run requires options")
	end
	for key in pairs(opts) do
		if
			key ~= "queries"
			and key ~= "lexical"
			and key ~= "vector"
			and key ~= "clock"
			and key ~= "memory"
			and key ~= "cancel"
		then
			fail("run contains unsupported field: " .. tostring(key))
		end
	end
	if type(opts.lexical) ~= "function" or type(opts.vector) ~= "function" then
		fail("run requires lexical and vector retrieval functions")
	end
	if opts.clock ~= nil and type(opts.clock) ~= "function" then
		fail("clock must be a function")
	end
	if opts.memory ~= nil and type(opts.memory) ~= "function" then
		fail("memory must be a function")
	end
	if opts.cancel ~= nil and type(opts.cancel) ~= "function" then
		fail("cancel must be a function")
	end
	local now = opts.clock or vim.uv.hrtime
	local memory = opts.memory or function()
		return collectgarbage("count")
	end
	local cancel = opts.cancel or function()
		return false
	end
	local value = queries(opts.queries)
	return {
		schema_version = M.schema_version,
		lexical = benchmark("lexical", opts.lexical, value, now, memory, cancel),
		vector = benchmark("vector", opts.vector, value, now, memory, cancel),
	}
end

return M
