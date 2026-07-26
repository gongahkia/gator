local compat = require("gator.compat")
local config = require("gator.config")
local consent = require("gator.telemetry.consent")
local M = { checks = {}, graph = require("gator.health.graph") }
local providers = {
	{ name = "aider", executable = "aider" },
	{ name = "amp", executable = "amp" },
	{ name = "cline", executable = "cline", cwd = true },
	{ name = "cursor", executable = "cursor-agent" },
	{ name = "codex", executable = "codex" },
	{ name = "claude", executable = "claude" },
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

local terminal_providers = { claude = true, codex = true, opencode = true, pi = true }
local terminal_capabilities = {
	claude = { "cli", "resume" },
	codex = { "rpc" },
	opencode = { "acp", "session_resume" },
	pi = { "stdio", "session_create", "session_resume" },
}
local pi_probe_timeout_ms = 10000

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
		or function(argv, directory, input, timeout_ms)
			local value = vim.system(argv, {
				cwd = directory,
				stdin = input,
				text = true,
				timeout = timeout_ms or 3000,
			}):wait()
			return { code = value.code, stdout = value.stdout or "" }
		end
	local cwd = opts.cwd or vim.fn.getcwd()
	local records = {}
	local function add(component, level, message, repair, readiness_state)
		table.insert(records, {
			component = component,
			level = level,
			message = message,
			repair = repair,
			readiness_state = readiness_state,
		})
	end
	local settings_ok, settings = pcall(config.resolve, opts.settings)
	local confirmations = settings_ok and settings.providers or {}
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
		local function probe_run(argv, input)
			return run(argv, cwd, input, provider.name == "pi" and pi_probe_timeout_ms or 3000)
		end
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
		local function probe_run(argv, input)
			return run(argv, cwd, input, provider.name == "pi" and pi_probe_timeout_ms or 3000)
		end
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
				"Install " .. provider.executable .. " to enable this adapter.",
				"indeterminate"
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
						or "Run the provider CLI manually, update it, then rerun :GatorHealth.",
					"indeterminate"
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
				local user_confirmed = type(confirmations[provider.name]) == "table"
					and confirmations[provider.name].user_confirmed == true
				local readiness_state = auth_ok and auth_result.authenticated and "detected"
					or user_confirmed and "user_confirmed"
					or "indeterminate"
				local authentication = auth_ok and auth_result.authenticated and "authentication probe passed"
					or user_confirmed and "readiness user-confirmed; credentials not verified: " .. (type(auth_reason) == "string" and auth_reason or "authentication status is unavailable")
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
					"Use only advertised capabilities, verify provider-native login, then rerun :GatorHealth.",
					readiness_state
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

function M.launch_catalog(opts)
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("launch catalog requires options")
	end
	for key in pairs(opts) do
		if key ~= "cwd" and key ~= "run" and key ~= "executable" and key ~= "pi_user_confirmed" then
			fail("launch catalog contains unsupported field: " .. tostring(key))
		end
	end
	local cwd = opts.cwd or vim.fn.getcwd()
	if type(cwd) ~= "string" or cwd == "" then
		fail("launch catalog cwd must be a non-empty string")
	end
	local executable = opts.executable or function(name)
		return vim.fn.executable(name) == 1
	end
	local run = opts.run
		or function(argv, directory, input, timeout_ms)
			local result = vim.system(argv, {
				cwd = directory,
				stdin = input,
				text = true,
				timeout = timeout_ms or 3000,
			}):wait()
			return { code = result.code, stdout = result.stdout or "" }
		end
	if type(executable) ~= "function" or type(run) ~= "function" then
		fail("launch catalog executable and run must be functions")
	end
	if opts.pi_user_confirmed ~= nil and type(opts.pi_user_confirmed) ~= "boolean" then
		fail("launch catalog pi_user_confirmed must be boolean")
	end
	local pi_user_confirmed = opts.pi_user_confirmed == true
	local records = {}
	for _, provider in ipairs(providers) do
		if terminal_providers[provider.name] then
			local record = { provider = provider.name, available = false, readiness_state = "indeterminate" }
			if not executable(provider.executable) then
				record.reason = provider.executable .. " is unavailable"
			else
				local loaded, adapter = pcall(require, provider_modules[provider.name])
				local probe
				if loaded then
					local args = {
						executable = provider.executable,
						run = function(argv, input)
							return run(argv, cwd, input, provider.name == "pi" and pi_probe_timeout_ms or 3000)
						end,
					}
					if provider.cwd then
						args.cwd = cwd
					end
					local ok, value = pcall(adapter.probe, args)
					probe = ok and value or nil
				end
				if type(probe) ~= "table" or not probe.available then
					record.reason = (probe and probe.reason) or "version or capability probe failed"
				elseif probe.supported == false then
					record.reason = "installed version is outside Gator's supported range"
				else
					for _, capability in ipairs(terminal_capabilities[provider.name]) do
						if type(probe.capabilities) ~= "table" or probe.capabilities[capability] ~= true then
							record.reason = "native terminal capability is unavailable: " .. capability
							break
						end
					end
					if provider.name == "pi" and not pi_user_confirmed then
						record.readiness_state = "detected"
						record.reason = "Pi is detected; explicit providers.pi.user_confirmed opt-in is required"
					elseif not record.reason then
						if provider.name == "pi" then
							record.available = true
							record.authentication = "user_confirmed"
							record.readiness_state = "user_confirmed"
							record.readiness_signals = { "CLI contract detected", "user-confirmed configuration" }
						else
							local ok, auth = pcall(adapter.auth, {
								executable = provider.executable,
								run = function(argv, input)
									return run(argv, cwd, input)
								end,
							})
							if ok and type(auth) == "table" and auth.authenticated then
								record.available = true
								record.readiness_state = "detected"
							else
								record.reason = (type(auth) == "table" and auth.reason)
									or "authentication is not verified"
							end
						end
					end
				end
			end
			table.insert(records, record)
		end
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
