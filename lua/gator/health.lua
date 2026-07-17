local compat = require("gator.compat")
local config = require("gator.config")
local consent = require("gator.telemetry.consent")
local M = { checks = {} }
local providers = { "amp", "codex", "claude", "gemini", "copilot", "opencode", "pi" }

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
		if key ~= "executable" and key ~= "cwd" and key ~= "run" and key ~= "settings" and key ~= "consent" then
			fail("readiness contains unsupported field: " .. tostring(key))
		end
	end
	if opts.executable ~= nil and type(opts.executable) ~= "function" then
		fail("readiness executable must be a function")
	end
	if opts.run ~= nil and type(opts.run) ~= "function" then
		fail("readiness run must be a function")
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
		or function(argv, cwd)
			local value = vim.system(argv, { cwd = cwd, text = true }):wait()
			return { code = value.code, stdout = value.stdout or "" }
		end
	local cwd = opts.cwd or vim.fn.getcwd()
	local records = {}
	local function add(component, level, message, repair)
		table.insert(records, { component = component, level = level, message = message, repair = repair })
	end
	for _, provider in ipairs(providers) do
		if executable(provider) then
			add("adapter." .. provider, "ok", "Adapter " .. provider .. " executable is available")
		else
			add(
				"adapter." .. provider,
				"warn",
				"Adapter " .. provider .. " executable is unavailable",
				"Install " .. provider .. " to enable this adapter."
			)
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
