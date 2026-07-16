local pack = require("gator.context.pack")
local M = {}

local severity_names = {
	[vim.diagnostic.severity.ERROR] = "error",
	[vim.diagnostic.severity.WARN] = "warning",
	[vim.diagnostic.severity.INFO] = "information",
	[vim.diagnostic.severity.HINT] = "hint",
}

local function fail(message)
	error("Gator diagnostic context: " .. message, 3)
end

local function buffer(value)
	if type(value) ~= "number" or value < 1 or value % 1 ~= 0 or not vim.api.nvim_buf_is_valid(value) then
		fail("buffer must be valid")
	end
	local name = vim.api.nvim_buf_get_name(value)
	local path = name ~= "" and vim.uv.fs_realpath(name) or nil
	if not path or vim.fn.filereadable(path) ~= 1 then
		fail("buffer must name an existing file")
	end
	return value, path
end

local function namespaces()
	local result = {}
	for name, id in pairs(vim.api.nvim_get_namespaces()) do
		result[id] = name
	end
	return result
end

local function source(value, names)
	if type(value.source) == "string" and value.source ~= "" then
		return value.source
	end
	local namespace = names[value.namespace]
	if type(namespace) ~= "string" or namespace == "" then
		fail("diagnostic must declare a source or known namespace")
	end
	return namespace
end

local function position(value, name)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail("diagnostic " .. name .. " must be a non-negative integer")
	end
	return value
end

local function details(value, path, names)
	if type(value) ~= "table" then
		fail("diagnostic must be a table")
	end
	local severity = severity_names[value.severity]
	if not severity then
		fail("diagnostic severity is unsupported")
	end
	if type(value.message) ~= "string" or value.message == "" then
		fail("diagnostic message must be a non-empty string")
	end
	local lnum = position(value.lnum, "lnum")
	local col = position(value.col, "col")
	local end_lnum = value.end_lnum == nil and lnum or position(value.end_lnum, "end_lnum")
	local end_col = value.end_col == nil and col or position(value.end_col, "end_col")
	local diagnostic_source = source(value, names)
	local code = value.code == nil and "" or tostring(value.code)
	local fingerprint = vim.fn.sha256(table.concat({
		path,
		diagnostic_source,
		severity,
		tostring(lnum),
		tostring(col),
		tostring(end_lnum),
		tostring(end_col),
		code,
		value.message,
	}, "\0"))
	return {
		fingerprint = fingerprint,
		source = diagnostic_source,
		severity = severity,
		message = value.message,
		code = code,
		location = {
			path = path,
			first_line = lnum + 1,
			first_column = col + 1,
			last_line = end_lnum + 1,
			last_column = end_col + 1,
		},
	}
end

local function diagnostics(buffer, path)
	local values = vim.diagnostic.get(buffer)
	if type(values) ~= "table" or not vim.islist(values) then
		fail("diagnostic API returned an invalid list")
	end
	local names = namespaces()
	local result = {}
	for index, value in ipairs(values) do
		result[index] = details(value, path, names)
	end
	table.sort(result, function(left, right)
		if left.location.first_line ~= right.location.first_line then
			return left.location.first_line < right.location.first_line
		end
		if left.location.first_column ~= right.location.first_column then
			return left.location.first_column < right.location.first_column
		end
		return left.fingerprint < right.fingerprint
	end)
	return result
end

function M.capture(opts)
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("options must be a table")
	end
	for key in pairs(opts) do
		if key ~= "buffer" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	local source, path = buffer(opts.buffer or vim.api.nvim_get_current_buf())
	local revision = vim.api.nvim_buf_get_changedtick(source)
	local result = {}
	for index, value in ipairs(diagnostics(source, path)) do
		local reference = "diagnostic://" .. path .. "#sha256=" .. value.fingerprint
		result[index] = {
			entry = pack.entry({
				id = "diagnostic-" .. value.fingerprint,
				kind = "diagnostic",
				ref = reference,
				provenance = { source = value.source, ref = path },
				trust = "provenance",
				token_estimate = { status = "unavailable", reason = "diagnostic content has not been provider-counted" },
				transfer = { eligible = true },
			}),
			buffer = source,
			revision = revision,
			fingerprint = value.fingerprint,
			source = value.source,
			severity = value.severity,
			message = value.message,
			code = value.code,
			location = value.location,
		}
	end
	return result
end

function M.is_stale(record)
	if
		type(record) ~= "table"
		or type(record.buffer) ~= "number"
		or type(record.revision) ~= "number"
		or type(record.fingerprint) ~= "string"
		or record.fingerprint == ""
	then
		fail("record must be returned by capture")
	end
	if
		not vim.api.nvim_buf_is_valid(record.buffer)
		or vim.api.nvim_buf_get_changedtick(record.buffer) ~= record.revision
	then
		return true
	end
	local _, path = buffer(record.buffer)
	for _, value in ipairs(diagnostics(record.buffer, path)) do
		if value.fingerprint == record.fingerprint then
			return false
		end
	end
	return true
end

return M
