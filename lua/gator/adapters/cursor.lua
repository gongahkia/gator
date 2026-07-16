local M = {}

local function fail(message)
	error("Gator Cursor probe: " .. message, 3)
end

local function run(argv)
	local value = vim.system(argv, { text = true }):wait()
	return { code = value.code, stdout = value.stdout or "" }
end

function M.probe(opts)
	opts = opts or {}
	if type(opts) ~= "table" or (opts.run ~= nil and type(opts.run) ~= "function") then
		fail("options must provide an optional run function")
	end
	for key in pairs(opts) do
		if key ~= "run" and key ~= "executable" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	local executable = opts.executable or "cursor-agent"
	if type(executable) ~= "string" or executable == "" then
		fail("executable must be a non-empty string")
	end
	local invoke = opts.run or run
	local ok, version = pcall(invoke, { executable, "--version" })
	if not ok or type(version) ~= "table" or version.code ~= 0 or type(version.stdout) ~= "string" then
		return { provider = "cursor", available = false, reason = "Cursor executable or version is unavailable" }
	end
	local found = version.stdout:match("(%d+%.%d+%.%d+)")
	if not found then
		return { provider = "cursor", available = false, reason = "Cursor version output is unrecognized" }
	end
	local help_ok, help = pcall(invoke, { executable, "--help" })
	local output = help_ok
			and type(help) == "table"
			and help.code == 0
			and type(help.stdout) == "string"
			and help.stdout
		or ""
	return {
		provider = "cursor",
		available = true,
		version = found,
		supported = true,
		capabilities = {
			structured_output = output:find("stream%-json") ~= nil,
			session_list = output:find("ls", 1, true) ~= nil,
			session_resume = output:find("--resume", 1, true) ~= nil,
			permission_prompt = output:find("--force", 1, true) ~= nil,
		},
	}
end

return M
