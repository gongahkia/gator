local filesystem = require("gator.core.filesystem")
local redact = require("gator.policy.redact")

local M = {
	api_version = 1,
	schema_version = 1,
	states = { ready = true, unavailable = true, failed = true, cancelled = true },
}

local function fail(message)
	error("Gator beta readiness: " .. redact.text(tostring(message)), 3)
end

local function timestamp(value, name)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail(name .. " must be a non-negative integer timestamp")
	end
	return value
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_.-]*$") then
		fail(name .. " must be a lowercase dotted identifier")
	end
	return value
end

local function root(value)
	if type(value) ~= "string" or value == "" or value:sub(1, 1) ~= "/" then
		fail("root must be an absolute path")
	end
	return vim.fs.normalize(value)
end

local function policy(value)
	if type(value) ~= "table" then
		fail("storage must be an object")
	end
	for key in pairs(value) do
		if key ~= "sharing" then
			fail("storage contains unsupported field: " .. tostring(key))
		end
	end
	if value.sharing ~= "local" then
		return nil, "beta readiness artifacts are unavailable because local-only storage is not configured"
	end
	return { sharing = "local" }
end

local function outcome(value, name)
	if type(value) ~= "table" then
		return nil, "check must return an object"
	end
	for key in pairs(value) do
		if key ~= "state" and key ~= "detail" then
			return nil, "check returned unsupported field: " .. tostring(key)
		end
	end
	if type(value.state) ~= "string" or not M.states[value.state] then
		return nil, "check returned an unsupported state"
	end
	if value.detail ~= nil and (type(value.detail) ~= "string" or value.detail == "") then
		return nil, "check detail must be non-empty text"
	end
	return { name = name, state = value.state, detail = value.detail and redact.text(value.detail) or nil }
end

local function aggregate(checks)
	for _, check in ipairs(checks) do
		if check.state == "failed" then
			return "failed"
		end
	end
	for _, check in ipairs(checks) do
		if check.state == "unavailable" then
			return "unavailable"
		end
	end
	for _, check in ipairs(checks) do
		if check.state == "cancelled" then
			return "cancelled"
		end
	end
	return "ready"
end

local function checks(value)
	if type(value) ~= "table" or not vim.islist(value) or #value == 0 then
		fail("checks must be a non-empty array")
	end
	local result, seen = {}, {}
	for index, check in ipairs(value) do
		if type(check) ~= "table" then
			fail("check " .. index .. " must be an object")
		end
		for key in pairs(check) do
			if key ~= "name" and key ~= "check" then
				fail("check " .. index .. " contains unsupported field: " .. tostring(key))
			end
		end
		local name = identifier(check.name, "check " .. index .. " name")
		if seen[name] then
			fail("checks contain duplicate names")
		end
		if type(check.check) ~= "function" then
			fail("check " .. index .. " check must be a function")
		end
		seen[name] = true
		result[index] = { name = name, check = check.check }
	end
	return result
end

local function cancellation(callback, name)
	if callback == nil then
		return false
	end
	local ok, value = pcall(callback, name)
	if not ok or type(value) ~= "boolean" then
		return nil, redact.text(ok and "cancel callback must return a boolean" or tostring(value))
	end
	return value
end

local function report(value)
	if type(value) ~= "table" or value.schema_version ~= M.schema_version then
		fail("report schema_version is unsupported")
	end
	for key in pairs(value) do
		if key ~= "schema_version" and key ~= "captured_at" and key ~= "state" and key ~= "checks" then
			fail("report contains unsupported field: " .. tostring(key))
		end
	end
	timestamp(value.captured_at, "report captured_at")
	if type(value.state) ~= "string" or not M.states[value.state] then
		fail("report state is unsupported")
	end
	if type(value.checks) ~= "table" or not vim.islist(value.checks) then
		fail("report checks must be an array")
	end
	local checks, names = {}, {}
	for index, check in ipairs(value.checks) do
		if type(check) ~= "table" then
			fail("report check " .. index .. " must be an object")
		end
		for key in pairs(check) do
			if key ~= "name" and key ~= "state" and key ~= "detail" then
				fail("report check " .. index .. " contains unsupported field: " .. tostring(key))
			end
		end
		local name = identifier(check.name, "report check " .. index .. " name")
		if names[name] then
			fail("report checks contain duplicate names")
		end
		local normalized, reason = outcome({ state = check.state, detail = check.detail }, name)
		if not normalized then
			fail("report check " .. index .. " is invalid: " .. reason)
		end
		names[name] = true
		table.insert(checks, normalized)
	end
	if value.state ~= aggregate(checks) then
		fail("report state does not match checks")
	end
	return {
		schema_version = M.schema_version,
		captured_at = value.captured_at,
		state = value.state,
		checks = checks,
	}
end

function M.verify(opts)
	if type(opts) ~= "table" then
		fail("verify requires options")
	end
	for key in pairs(opts) do
		if key ~= "checks" and key ~= "captured_at" and key ~= "cancel" then
			fail("verify contains unsupported field: " .. tostring(key))
		end
	end
	if opts.cancel ~= nil and type(opts.cancel) ~= "function" then
		fail("cancel must be a function")
	end
	local configured = checks(opts.checks)
	local records = {}
	for _, entry in ipairs(configured) do
		local cancelled, reason = cancellation(opts.cancel, entry.name)
		if cancelled == nil then
			table.insert(records, { name = entry.name, state = "failed", detail = reason })
			break
		end
		if cancelled then
			table.insert(
				records,
				{ name = entry.name, state = "cancelled", detail = "beta readiness verification was cancelled" }
			)
			break
		end
		local ok, value = pcall(entry.check)
		local record, detail
		if ok then
			record, detail = outcome(value, entry.name)
		else
			detail = tostring(value)
		end
		if record then
			table.insert(records, record)
		else
			table.insert(records, { name = entry.name, state = "failed", detail = redact.text(detail) })
		end
	end
	return report({
		schema_version = M.schema_version,
		captured_at = opts.captured_at or os.time(),
		state = aggregate(records),
		checks = records,
	})
end

function M.write(opts)
	if type(opts) ~= "table" then
		fail("write requires options")
	end
	for key in pairs(opts) do
		if key ~= "report" and key ~= "root" and key ~= "storage" and key ~= "filesystem" then
			fail("write contains unsupported field: " .. tostring(key))
		end
	end
	if opts.filesystem ~= nil and not filesystem.is(opts.filesystem) then
		fail("filesystem must be a Gator filesystem boundary")
	end
	local value = report(opts.report)
	local storage, unavailable = policy(opts.storage)
	local base = root(opts.root or (vim.fn.stdpath("state") .. "/gator"))
	local directory = base .. "/beta-readiness"
	local path = directory .. "/report.json"
	local failure_path = directory .. "/failure.json"
	if not storage then
		return { state = "unavailable", path = path, reason = unavailable }
	end
	if value.state == "cancelled" then
		return { state = "cancelled", path = path, reason = "beta readiness verification was cancelled" }
	end
	local boundary = opts.filesystem or filesystem.new()
	local temporary = {}
	local function write(name, content)
		local candidate = name .. ".tmp-" .. vim.uv.hrtime()
		table.insert(temporary, candidate)
		if not boundary:write(candidate, vim.json.encode(content)) then
			fail("cannot write beta readiness artifact")
		end
		if not boundary:rename(candidate, name) then
			boundary:remove(candidate)
			fail("cannot replace beta readiness artifact")
		end
	end
	local ok, reason = pcall(function()
		if not boundary:mkdir(directory) then
			fail("cannot create beta readiness directory")
		end
		write(path, value)
		if value.state == "ready" then
			if boundary:readable(failure_path) and not boundary:remove(failure_path) then
				fail("cannot remove stale beta readiness failure report")
			end
			return
		end
		local failures = {}
		for _, check in ipairs(value.checks) do
			if check.state ~= "ready" then
				table.insert(failures, check)
			end
		end
		write(failure_path, {
			schema_version = M.schema_version,
			captured_at = value.captured_at,
			state = value.state,
			failures = failures,
		})
	end)
	if not ok then
		for _, candidate in ipairs(temporary) do
			pcall(boundary.remove, boundary, candidate)
		end
		return { state = "failed", path = path, reason = redact.text(tostring(reason)) }
	end
	return {
		state = value.state,
		path = path,
		failure_path = value.state == "ready" and nil or failure_path,
		report = vim.deepcopy(value),
	}
end

return M
