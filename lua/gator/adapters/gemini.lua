local M = {}

M.range = { minimum = { 0, 46, 0 }, maximum = { 0, 46, 999 } }

local function fail(message)
	error("Gator Gemini probe: " .. message, 3)
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

function M.probe(opts)
	opts = opts or {}
	if type(opts) ~= "table" or (opts.run ~= nil and type(opts.run) ~= "function") then
		fail("options must provide an optional run function")
	end
	local executable = opts.executable or "gemini"
	local invoke = opts.run
		or function(argv)
			local result = vim.system(argv, { text = true }):wait()
			return { code = result.code, stdout = result.stdout or "" }
		end
	local range = opts.range or M.range
	local minimum, maximum = version(range.minimum, "range.minimum"), version(range.maximum, "range.maximum")
	local ok, result = pcall(invoke, { executable, "--version" })
	if not ok or type(result) ~= "table" or result.code ~= 0 or type(result.stdout) ~= "string" then
		return { provider = "gemini", available = false, reason = "Gemini executable or version is unavailable" }
	end
	local major, minor, patch = result.stdout:match("(%d+)%.(%d+)%.(%d+)")
	if not major then
		return { provider = "gemini", available = false, reason = "Gemini version output is unrecognized" }
	end
	local found = { tonumber(major), tonumber(minor), tonumber(patch) }
	local help_ok, help = pcall(invoke, { executable, "--help" })
	local output = help_ok
			and type(help) == "table"
			and help.code == 0
			and type(help.stdout) == "string"
			and help.stdout
		or ""
	return {
		provider = "gemini",
		available = true,
		version = found,
		supported = compare(found, minimum) >= 0 and compare(found, maximum) <= 0,
		capabilities = {
			acp = output:find("--acp", 1, true) ~= nil,
			structured_output = output:find("stream-json", 1, true) ~= nil,
			sessions = output:find("--list-sessions", 1, true) ~= nil and output:find("--resume", 1, true) ~= nil,
		},
	}
end

function M.launch(opts)
	if type(opts) ~= "table" or type(opts.manager) ~= "table" or type(opts.manager.launch) ~= "function" then
		fail("launch requires a process manager")
	end
	for key in pairs(opts) do
		if key ~= "manager" and key ~= "id" and key ~= "cwd" and key ~= "args" and key ~= "executable" then
			fail("launch contains unsupported field: " .. tostring(key))
		end
	end
	if type(opts.id) ~= "string" or not opts.id:match("^[a-z][a-z0-9_-]*$") then
		fail("id must be a lowercase identifier")
	end
	if type(opts.cwd) ~= "string" or opts.cwd == "" then
		fail("cwd must be a non-empty string")
	end
	local cwd = vim.uv.fs_realpath(opts.cwd)
	if not cwd or vim.fn.isdirectory(cwd) ~= 1 then
		fail("cwd must resolve to a directory")
	end
	local executable = opts.executable or "gemini"
	if type(executable) ~= "string" or executable == "" then
		fail("executable must be a non-empty string")
	end
	local args = opts.args or {}
	if type(args) ~= "table" or not vim.islist(args) then
		fail("args must be an array")
	end
	local argv = { executable }
	for index, argument in ipairs(args) do
		if type(argument) ~= "string" or argument == "" then
			fail("argument " .. index .. " must be a non-empty string")
		end
		table.insert(argv, argument)
	end
	return opts.manager:launch({ id = opts.id, command = argv, cwd = cwd })
end

return M
