local redact = require("gator.policy.redact")
local runtime = require("gator.core.runtime")

local M = {
	api_version = 1,
	schema_version = 1,
	states = { ready = true, degraded = true, unavailable = true, failed = true, cancelled = true },
}

local function fail(message)
	error("Gator runtime diagnostics: " .. redact.text(tostring(message)), 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function diagnostics(value)
	if type(value) ~= "table" or not vim.islist(value) then
		fail("diagnostics must be an array")
	end
	local result, names = {}, {}
	for index, value in ipairs(value) do
		if type(value) ~= "table" then
			fail("diagnostic " .. index .. " must be an object")
		end
		for key in pairs(value) do
			if key ~= "name" and key ~= "check" then
				fail("diagnostic " .. index .. " contains unsupported field: " .. tostring(key))
			end
		end
		local name = identifier(value.name, "diagnostic " .. index .. " name")
		if names[name] then
			fail("diagnostics contain duplicate names")
		end
		if type(value.check) ~= "function" then
			fail("diagnostic " .. index .. " check must be a function")
		end
		names[name] = true
		result[index] = { name = name, check = value.check }
	end
	return result
end

local function diagnostic(name, value)
	if type(value) ~= "table" then
		return nil, "diagnostic must return an object"
	end
	for key in pairs(value) do
		if key ~= "state" and key ~= "detail" then
			return nil, "diagnostic returned unsupported field: " .. tostring(key)
		end
	end
	if type(value.state) ~= "string" or not M.states[value.state] then
		return nil, "diagnostic returned an unsupported state"
	end
	if value.detail ~= nil and (type(value.detail) ~= "string" or value.detail == "") then
		return nil, "diagnostic detail must be non-empty text"
	end
	return { name = name, state = value.state, detail = value.detail and redact.text(value.detail) or nil }
end

local function aggregate(services, records)
	for _, record in ipairs(records) do
		if record.state == "failed" then
			return "failed"
		end
	end
	if #services == 0 then
		return "unavailable"
	end
	local cancelled, degraded = true, false
	for _, service in ipairs(services) do
		if service.state == "failed" then
			return "failed"
		end
		cancelled = cancelled and service.state == "cancelled"
		degraded = degraded or service.state ~= "running"
	end
	for _, record in ipairs(records) do
		degraded = degraded or record.state == "unavailable" or record.state == "degraded"
	end
	if cancelled then
		return "cancelled"
	end
	return degraded and "degraded" or "ready"
end

function M.capture(opts)
	if type(opts) ~= "table" or not runtime.is(opts.runtime) then
		fail("capture requires a runtime owner")
	end
	for key in pairs(opts) do
		if key ~= "runtime" and key ~= "diagnostics" and key ~= "cancel" then
			fail("capture contains unsupported field: " .. tostring(key))
		end
	end
	if opts.cancel ~= nil and type(opts.cancel) ~= "function" then
		fail("cancel must be a function")
	end
	local configured = diagnostics(opts.diagnostics or {})
	local services = opts.runtime:services()
	if opts.cancel then
		local ok, cancelled = pcall(opts.cancel)
		if not ok or type(cancelled) ~= "boolean" then
			return {
				schema_version = M.schema_version,
				state = "failed",
				services = services,
				diagnostics = {},
				reason = redact.text(ok and "cancel callback must return a boolean" or tostring(cancelled)),
			}
		end
		if cancelled then
			return {
				schema_version = M.schema_version,
				state = "cancelled",
				services = services,
				diagnostics = {},
				reason = "runtime diagnostic snapshot was cancelled",
			}
		end
	end
	local records = {}
	for _, value in ipairs(configured) do
		local ok, result = pcall(value.check)
		local record, reason
		if ok then
			record, reason = diagnostic(value.name, result)
		else
			reason = tostring(result)
		end
		if record then
			table.insert(records, record)
		else
			table.insert(records, { name = value.name, state = "failed", detail = redact.text(reason) })
		end
	end
	table.sort(records, function(left, right)
		return left.name < right.name
	end)
	return {
		schema_version = M.schema_version,
		state = aggregate(services, records),
		services = services,
		diagnostics = records,
	}
end

return M
