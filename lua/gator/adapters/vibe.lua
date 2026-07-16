local M = {}
local function fail(message)
	error("Gator Vibe probe: " .. message, 3)
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
	local executable = opts.executable or "vibe"
	if type(executable) ~= "string" or executable == "" then
		fail("executable must be a non-empty string")
	end
	local invoke = opts.run
		or function(argv)
			local value = vim.system(argv, { text = true }):wait()
			return { code = value.code, stdout = value.stdout or "" }
		end
	local ok, version = pcall(invoke, { executable, "--version" })
	if not ok or type(version) ~= "table" or version.code ~= 0 or type(version.stdout) ~= "string" then
		return { provider = "vibe", available = false, reason = "Vibe executable or version is unavailable" }
	end
	local found = version.stdout:match("(%d+%.%d+%.%d+)")
	if not found then
		return { provider = "vibe", available = false, reason = "Vibe version output is unrecognized" }
	end
	local help_ok, help = pcall(invoke, { executable, "--help" })
	local output = help_ok
			and type(help) == "table"
			and help.code == 0
			and type(help.stdout) == "string"
			and help.stdout
		or ""
	return {
		provider = "vibe",
		available = true,
		version = found,
		supported = true,
		capabilities = {
			acp = output:find("acp", 1, true) ~= nil,
			structured_output = output:find("--output", 1, true) ~= nil,
			plan = output:find("--agent plan", 1, true) ~= nil,
			approval = output:find("accept%-edits") ~= nil,
		},
	}
end

return M
