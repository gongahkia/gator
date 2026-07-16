local M = {}

M.range = { minimum = { 0, 144, 0 }, maximum = { 0, 144, 999 } }

local function fail(message)
	error("Gator Codex probe: " .. message, 3)
end

local function version(value, name)
	if type(value) ~= "table" or #value ~= 3 then
		fail(name .. " must be a three-part version")
	end
	for index, part in ipairs(value) do
		if type(part) ~= "number" or part < 0 or part % 1 ~= 0 then
			fail(name .. " part " .. index .. " must be a non-negative integer")
		end
	end
	return value
end

local function compare(left, right)
	for index = 1, 3 do
		if left[index] ~= right[index] then
			return left[index] < right[index] and -1 or 1
		end
	end
	return 0
end

local function run(argv)
	local result = vim.system(argv, { text = true }):wait()
	return { code = result.code, stdout = result.stdout or "" }
end

function M.probe(opts)
	opts = opts or {}
	if type(opts) ~= "table" or (opts.run ~= nil and type(opts.run) ~= "function") then
		fail("options must provide an optional run function")
	end
	for key in pairs(opts) do
		if key ~= "run" and key ~= "executable" and key ~= "range" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	local executable = opts.executable or "codex"
	if type(executable) ~= "string" or executable == "" then
		fail("executable must be a non-empty string")
	end
	local supported = opts.range or M.range
	if type(supported) ~= "table" then
		fail("range must be a table")
	end
	local minimum, maximum = version(supported.minimum, "range.minimum"), version(supported.maximum, "range.maximum")
	if compare(minimum, maximum) > 0 then
		fail("range minimum cannot exceed maximum")
	end
	local invoke = opts.run or run
	local ok, result = pcall(invoke, { executable, "--version" })
	if not ok or type(result) ~= "table" or result.code ~= 0 or type(result.stdout) ~= "string" then
		return { provider = "codex", available = false, reason = "Codex executable or version is unavailable" }
	end
	local major, minor, patch = result.stdout:match("codex%-cli%s+(%d+)%.(%d+)%.(%d+)")
	if not major then
		return { provider = "codex", available = false, reason = "Codex version output is unrecognized" }
	end
	local found = { tonumber(major), tonumber(minor), tonumber(patch) }
	local app_ok, app = pcall(invoke, { executable, "app-server", "--help" })
	local rpc = app_ok
		and type(app) == "table"
		and app.code == 0
		and type(app.stdout) == "string"
		and app.stdout:find("stdio://", 1, true) ~= nil
	return {
		provider = "codex",
		available = true,
		version = found,
		supported = compare(found, minimum) >= 0 and compare(found, maximum) <= 0,
		capabilities = { cli = true, rpc = rpc },
	}
end

return M
