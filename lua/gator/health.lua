local compat = require("gator.compat")
local config = require("gator.config")
local M = { checks = {} }

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

return M
