local errors = require("gator.error")
local M = { manifest_version = 1, minimum = { major = 0, minor = 11, patch = 0 } }

local function fail(message)
	error("invalid Gator compatibility report: " .. message, 3)
end

local function version_string(version)
	return table.concat({ version.major, version.minor, version.patch }, ".")
end

local function validate_version(version)
	if type(version) ~= "table" then
		fail("version must be a table")
	end
	for _, key in ipairs({ "major", "minor", "patch" }) do
		if type(version[key]) ~= "number" or version[key] < 0 or version[key] % 1 ~= 0 then
			fail("version." .. key .. " must be a non-negative integer")
		end
	end
	return version
end

local function supported(version)
	if version.major ~= M.minimum.major then
		return version.major > M.minimum.major
	end
	if version.minor ~= M.minimum.minor then
		return version.minor > M.minimum.minor
	end
	return version.patch >= M.minimum.patch
end

local function capabilities()
	return {
		health = type(vim.health) == "table",
		process = type(vim.system) == "function",
	}
end

function M.inspect(opts)
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("options must be a table")
	end
	local version = validate_version(opts.version or vim.version())
	local available = opts.capabilities or capabilities()
	if type(available) ~= "table" then
		fail("capabilities must be a table")
	end
	local degraded = {}

	for name, is_available in pairs(available) do
		if type(name) ~= "string" or type(is_available) ~= "boolean" then
			fail("capabilities must map strings to booleans")
		end
		if not is_available then
			table.insert(degraded, name)
		end
	end
	table.sort(degraded)
	return {
		version = version,
		supported = supported(version),
		capabilities = available,
		degraded = degraded,
	}
end

function M.manifest(opts)
	local report = M.inspect(opts)
	return {
		schema_version = M.manifest_version,
		product = "gator",
		neovim = {
			current = vim.deepcopy(report.version),
			minimum = vim.deepcopy(M.minimum),
			supported = report.supported,
		},
		capabilities = vim.deepcopy(report.capabilities),
		degraded = vim.deepcopy(report.degraded),
	}
end

function M.manifest_json(opts)
	return vim.json.encode(M.manifest(opts))
end

function M.require_supported(report)
	report = report or M.inspect()
	if type(report) ~= "table" or type(report.supported) ~= "boolean" or type(report.version) ~= "table" then
		fail("report is incomplete")
	end
	validate_version(report.version)
	if not report.supported then
		errors.raise(
			errors.new(
				"compatibility.neovim_unsupported",
				"Neovim " .. version_string(report.version) .. " is unsupported",
				{
					remedy = "Upgrade Neovim to " .. version_string(M.minimum) .. " or newer.",
				}
			)
		)
	end
	return report
end

function M.summary(report)
	report = report or M.inspect()
	if
		type(report) ~= "table"
		or type(report.supported) ~= "boolean"
		or type(report.version) ~= "table"
		or type(report.degraded) ~= "table"
	then
		fail("report is incomplete")
	end
	validate_version(report.version)
	local lines = { "Neovim: " .. version_string(report.version) }

	if report.supported then
		table.insert(lines, "Compatibility: supported")
	else
		table.insert(lines, "Compatibility: unsupported (requires " .. version_string(M.minimum) .. "+)")
	end
	if #report.degraded == 0 then
		table.insert(lines, "Optional capabilities: complete")
	else
		table.insert(lines, "Optional capabilities degraded: " .. table.concat(report.degraded, ", "))
	end
	return lines
end

return M
