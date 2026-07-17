local compat = require("gator.compat")
local config = require("gator.config")
local consent = require("gator.telemetry.consent")
local M = { checks = {} }
local providers = {
	{ name = "aider", executable = "aider" },
	{ name = "amp", executable = "amp" },
	{ name = "cline", executable = "cline", cwd = true },
	{ name = "cursor", executable = "cursor-agent" },
	{ name = "codex", executable = "codex" },
	{ name = "claude", executable = "claude" },
	{ name = "droid", executable = "droid" },
	{ name = "gemini", executable = "gemini" },
	{ name = "goose", executable = "goose" },
	{ name = "kimi", executable = "kimi" },
	{ name = "copilot", executable = "copilot" },
	{ name = "opencode", executable = "opencode", cwd = true },
	{ name = "pi", executable = "pi", cwd = true },
	{ name = "vibe", executable = "vibe" },
}
local provider_modules = {}
for _, provider in ipairs(providers) do
	provider_modules[provider.name] = "gator.adapters." .. provider.name
end

local function fail(message)
	error("invalid Gator health check: " .. message, 3)
end

local function validate_name(name)
	if type(name) ~= "string" or not name:match("^[a-z][a-z0-9_.-]*$") then
		fail("name must be a lowercase dotted identifier")
	end
end

local function validate_reporter(reporter)
	if type(reporter) ~= "table" then
		fail("reporter must be a table")
	end
	for _, method in ipairs({ "start", "ok", "warn", "error" }) do
		if type(reporter[method]) ~= "function" then
			fail("reporter." .. method .. " must be a function")
		end
	end
end

function M.register(name, check)
	validate_name(name)
	if type(check) ~= "function" then
		fail("check must be a function")
	end
	if M.checks[name] then
		fail("check is already registered: " .. name)
	end
	M.checks[name] = check
end

function M.unregister(name)
	validate_name(name)
	if not M.checks[name] then
		return false
	end
	M.checks[name] = nil
	return true
end

function M.run(reporter)
	validate_reporter(reporter)
	local names = vim.tbl_keys(M.checks)
	table.sort(names)

	for _, name in ipairs(names) do
		reporter.start("Gator " .. name)
		local ok, err = xpcall(function()
			M.checks[name](reporter)
		end, debug.traceback)
		if not ok then
			reporter.error(
				"Health check failed: " .. name .. "\n" .. err,
				"Correct the reported configuration and rerun :checkhealth gator."
			)
		end
	end
end

function M.check()
	if type(vim.health) ~= "table" then
		fail(
			"vim.health is unavailable; upgrade Neovim to 0."
				.. compat.minimum.minor
				.. "."
				.. compat.minimum.patch
				.. " or newer"
		)
	end
	M.run(vim.health)
end

function M.readiness(opts)
	if opts == nil then
		opts = {}
	end
	if type(opts) ~= "table" then
		fail("readiness requires options")
	end
	for key in pairs(opts) do
		if
			key ~= "executable"
			and key ~= "cwd"
			and key ~= "run"
			and key ~= "settings"
			and key ~= "consent"
			and key ~= "probe"
			and key ~= "auth"
		then
			fail("readiness contains unsupported field: " .. tostring(key))
		end
	end
	if opts.executable ~= nil and type(opts.executable) ~= "function" then
		fail("readiness executable must be a function")
	end
	if opts.run ~= nil and type(opts.run) ~= "function" then
		fail("readiness run must be a function")
	end
	if opts.probe ~= nil and type(opts.probe) ~= "function" then
		fail("readiness probe must be a function")
	end
	if opts.auth ~= nil and type(opts.auth) ~= "function" then
		fail("readiness auth must be a function")
	end
	if opts.cwd ~= nil and (type(opts.cwd) ~= "string" or opts.cwd == "") then
		fail("readiness cwd must be a non-empty string")
	end
	if opts.consent ~= nil and (type(opts.consent) ~= "table" or type(opts.consent.status) ~= "function") then
		fail("readiness consent must expose status")
	end
	local executable = opts.executable or function(name)
		return vim.fn.executable(name) == 1
	end
	local run = opts.run
		or function(argv, directory, input)
			local value = vim.system(argv, { cwd = directory, stdin = input, text = true, timeout = 3000 }):wait()
			return { code = value.code, stdout = value.stdout or "" }
		end
	local cwd = opts.cwd or vim.fn.getcwd()
	local function probe_run(argv, input)
		return run(argv, cwd, input)
	end
	local records = {}
	local function add(component, level, message, repair)
		table.insert(records, { component = component, level = level, message = message, repair = repair })
	end
	local function version(value)
		if type(value) == "string" then
			return value
		end
		if type(value) == "table" and #value == 3 then
			return table.concat(value, ".")
		end
		return nil
	end
	local function probe(provider, adapter)
		if opts.probe then
			return opts.probe(provider.name, provider.executable, probe_run)
		end
		local args = { executable = provider.executable, run = probe_run }
		if provider.cwd then
			args.cwd = cwd
		end
		return adapter.probe(args)
	end
	local function auth(provider, adapter)
		if opts.auth then
			return opts.auth(provider.name, provider.executable, probe_run)
		end
		if type(adapter.auth) ~= "function" then
			return {
				provider = provider.name,
				authenticated = false,
				reason = "This provider does not expose a non-interactive authentication-status probe",
			}
		end
		if provider.name == "codex" or provider.name == "claude" or provider.name == "opencode" then
			return adapter.auth({ executable = provider.executable, run = probe_run })
		end
		return adapter.auth()
	end
	for _, provider in ipairs(providers) do
		if not executable(provider.executable) then
			add(
				"adapter." .. provider.name,
				"warn",
				"Adapter " .. provider.name .. " executable " .. provider.executable .. " is unavailable",
				"Install " .. provider.executable .. " to enable this adapter."
			)
		else
			local loaded, adapter = pcall(require, provider_modules[provider.name])
			local ok, result = false, nil
			if loaded then
				ok, result = pcall(probe, provider, adapter)
			end
			if not ok or type(result) ~= "table" or result.available ~= true then
				add(
					"adapter." .. provider.name,
					"warn",
					"Adapter " .. provider.name .. " version or capability probe failed",
					(ok and result and result.reason)
						or "Run the provider CLI manually, update it, then rerun :GatorHealth."
				)
			else
				local authenticated, auth_result = pcall(auth, provider, adapter)
				local auth_ok = authenticated
					and type(auth_result) == "table"
					and type(auth_result.authenticated) == "boolean"
				local auth_reason = auth_ok and auth_result.reason or "authentication-status probe failed"
				local detected = version(result.version)
				local capability = result.supported == false and "outside Gator's supported capability range"
					or "capability probe passed"
				local authentication = auth_ok and auth_result.authenticated and "authentication probe passed"
					or "authentication not verified: "
						.. (type(auth_reason) == "string" and auth_reason or "authentication status is unavailable")
				add(
					"adapter." .. provider.name,
					result.supported ~= false and auth_ok and auth_result.authenticated and "ok" or "warn",
					"Adapter "
						.. provider.name
						.. " "
						.. capability
						.. (detected and " " .. detected or "")
						.. "; "
						.. authentication,
					"Use only advertised capabilities, verify provider-native login, then rerun :GatorHealth."
				)
			end
		end
	end
	local git = executable("git")
	if git then
		add("git", "ok", "Git executable is available")
	else
		add("git", "error", "Git executable is unavailable", "Install Git and ensure it is on PATH.")
	end
	if executable("gator-index") then
		add("indexer", "ok", "Optional gator-index sidecar is available")
	else
		add(
			"indexer",
			"warn",
			"Optional gator-index sidecar is unavailable",
			"Build gator-index to enable local vector indexing."
		)
	end
	local workspace_ok, workspace_result = false, nil
	if git then
		workspace_ok, workspace_result = pcall(run, { "git", "rev-parse", "--is-inside-work-tree" }, cwd)
	end
	if
		workspace_ok
		and type(workspace_result) == "table"
		and workspace_result.code == 0
		and vim.trim(workspace_result.stdout or "") == "true"
	then
		add("workspace", "ok", "Current directory is a Git workspace")
	else
		add(
			"workspace",
			"warn",
			"Current directory is not a Git workspace",
			"Open a Git workspace before worktree actions."
		)
	end
	local policy_ok, policy = pcall(config.resolve, opts.settings)
	if policy_ok and type(policy) == "table" then
		add("policy", "ok", "Policy configuration is valid")
	else
		add("policy", "error", "Policy configuration is invalid", tostring(policy))
	end
	local telemetry = (opts.consent or consent):status()
	if type(telemetry) ~= "table" or type(telemetry.enabled) ~= "boolean" then
		fail("telemetry consent returned an invalid status")
	end
	if telemetry.enabled then
		add("telemetry", "ok", "Telemetry collection and transmission are explicitly enabled")
	else
		add("telemetry", "ok", "Telemetry collection and transmission are disabled pending explicit consent")
	end
	return records
end

M.register("compatibility", function(report)
	local status = compat.inspect()
	if status.supported then
		report.ok(
			"Neovim "
				.. status.version.major
				.. "."
				.. status.version.minor
				.. "."
				.. status.version.patch
				.. " is supported"
		)
	else
		report.error("Neovim is unsupported", "Upgrade to Neovim 0.11.0 or newer.")
	end
	if #status.degraded == 0 then
		report.ok("Optional capabilities are complete")
	else
		report.warn("Optional capabilities are degraded: " .. table.concat(status.degraded, ", "))
	end
end)

M.register("configuration", function(report)
	local ok, err = pcall(config.resolve)
	if ok then
		report.ok("Default Gator configuration is valid")
	else
		report.error("Default Gator configuration is invalid", err)
	end
end)

M.register("dependencies", function(report)
	if vim.fn.executable("git") == 1 then
		report.ok("Git is available")
	else
		report.error("Git is unavailable", "Install Git and ensure it is on PATH.")
	end
	if vim.fn.executable("gator-index") == 1 then
		report.ok("Optional gator-index sidecar is available")
	else
		report.warn(
			"Optional gator-index sidecar is unavailable",
			"Build gator-index or configure its executable path when indexer support is needed."
		)
	end
end)

M.register("readiness", function(report)
	for _, record in ipairs(M.readiness()) do
		if record.level == "ok" then
			report.ok(record.message)
		elseif record.level == "warn" then
			report.warn(record.message, record.repair)
		else
			report.error(record.message, record.repair)
		end
	end
end)

return M
