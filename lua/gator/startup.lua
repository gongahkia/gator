local deferred = { "gator.adapters", "gator.indexer", "gator.github" }
local M = { name = "startup", api_version = 1, deferred_modules = vim.deepcopy(deferred) }

local function fail(message)
	error("Gator startup: " .. message, 3)
end

local function clock(value)
	if type(value) ~= "number" or value < 0 then
		fail("clock must return a non-negative number")
	end
	return value
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
