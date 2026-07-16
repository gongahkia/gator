local M = { api_version = 1, schema_version = 1 }
local names = { "startup", "context", "stream", "worktree", "diff", "indexer" }

local function fail(message)
	error("Gator performance suite: " .. message, 3)
end

local function number(value, name)
	if type(value) ~= "number" or value < 0 then
		fail(name .. " must return a non-negative number")
	end
	return value
end

function M.run(opts)
	if type(opts) ~= "table" or type(opts.cases) ~= "table" then
		fail("run requires cases")
	end
	for key in pairs(opts) do
		if key ~= "cases" and key ~= "clock" and key ~= "memory" and key ~= "path" then
			fail("run contains unsupported field: " .. tostring(key))
		end
	end
	local clock, memory = opts.clock or vim.uv.hrtime, opts.memory or function()
		return collectgarbage("count")
	end
	if type(clock) ~= "function" or type(memory) ~= "function" then
		fail("clock and memory must be functions")
	end
	local metrics = {}
	for _, name in ipairs(names) do
		if type(opts.cases[name]) ~= "function" then
			fail("cases." .. name .. " must be a function")
		end
		local started, before = number(clock(), "clock"), number(memory(), "memory")
		local ok = pcall(opts.cases[name])
		if not ok then
			fail(name .. " benchmark failed")
		end
		local elapsed, after = number(clock(), "clock") - started, number(memory(), "memory")
		if elapsed < 0 then
			fail("clock moved backwards")
		end
		table.insert(metrics, { name = name, duration_ms = elapsed / 1000000, memory_kb_delta = after - before })
	end
	local report = { schema_version = M.schema_version, metrics = metrics }
	if opts.path then
		if type(opts.path) ~= "string" or opts.path == "" then
			fail("path must be a non-empty string")
		end
		local parent = vim.fn.fnamemodify(opts.path, ":h")
		if vim.fn.isdirectory(parent) ~= 1 or vim.fn.writefile({ vim.json.encode(report) }, opts.path) ~= 0 then
			fail("cannot write benchmark artifact")
		end
	end
	return report
end

return M
