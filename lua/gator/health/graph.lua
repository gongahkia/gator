local redact = require("gator.policy.redact")

local M = { statuses = { ready = true, unavailable = true, failed = true, cancelled = true } }

local function fail(message)
	error("Gator health graph: " .. redact.text(tostring(message)), 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_.-]*$") then
		fail(name .. " must be a lowercase dotted identifier")
	end
	return value
end

local function component(value, index)
	if type(value) ~= "table" then
		fail("component " .. index .. " must be an object")
	end
	for key in pairs(value) do
		if key ~= "name" and key ~= "depends_on" and key ~= "check" then
			fail("component " .. index .. " contains unsupported field: " .. tostring(key))
		end
	end
	if type(value.depends_on) ~= "table" or not vim.islist(value.depends_on) then
		fail("component " .. index .. " depends_on must be an array")
	end
	if type(value.check) ~= "function" then
		fail("component " .. index .. " check must be a function")
	end
	local dependencies, seen = {}, {}
	for dependency_index, name in ipairs(value.depends_on) do
		name = identifier(name, "component " .. index .. " dependency " .. dependency_index)
		if seen[name] then
			fail("component " .. index .. " dependency is duplicated: " .. name)
		end
		seen[name] = true
		table.insert(dependencies, name)
	end
	table.sort(dependencies)
	return {
		name = identifier(value.name, "component " .. index .. " name"),
		depends_on = dependencies,
		check = value.check,
	}
end

local function status(value)
	if type(value) ~= "table" then
		return nil, "check must return a status object"
	end
	for key in pairs(value) do
		if key ~= "status" and key ~= "detail" then
			return nil, "check returned unsupported field: " .. tostring(key)
		end
	end
	if type(value.status) ~= "string" or not M.statuses[value.status] then
		return nil, "check returned an unavailable status"
	end
	if value.detail ~= nil and (type(value.detail) ~= "string" or value.detail == "") then
		return nil, "check detail must be a non-empty string"
	end
	return { status = value.status, detail = value.detail and redact.text(value.detail) or nil }
end

local function order(by_name)
	local names = vim.tbl_keys(by_name)
	table.sort(names)
	local visiting, visited, result = {}, {}, {}
	local function visit(name)
		if visited[name] then
			return
		end
		if visiting[name] then
			fail("dependency graph contains a cycle at " .. name)
		end
		visiting[name] = true
		for _, dependency in ipairs(by_name[name].depends_on) do
			if by_name[dependency] then
				visit(dependency)
			end
		end
		visiting[name] = nil
		visited[name] = true
		table.insert(result, by_name[name])
	end
	for _, name in ipairs(names) do
		visit(name)
	end
	return result
end

function M.evaluate(opts)
	if type(opts) ~= "table" then
		fail("evaluate requires options")
	end
	for key in pairs(opts) do
		if key ~= "components" and key ~= "cancel" then
			fail("evaluate contains unsupported field: " .. tostring(key))
		end
	end
	if type(opts.components) ~= "table" or not vim.islist(opts.components) then
		fail("evaluate requires a component array")
	end
	if opts.cancel ~= nil and type(opts.cancel) ~= "function" then
		fail("evaluate cancel must be a function")
	end
	local by_name = {}
	for index, value in ipairs(opts.components) do
		local entry = component(value, index)
		if by_name[entry.name] then
			fail("component is duplicated: " .. entry.name)
		end
		by_name[entry.name] = entry
	end
	local result, results = {}, {}
	for _, entry in ipairs(order(by_name)) do
		local record = { name = entry.name, depends_on = vim.deepcopy(entry.depends_on) }
		local blocked
		for _, dependency in ipairs(entry.depends_on) do
			local dependency_record = results[dependency]
			if not dependency_record then
				blocked = "dependency " .. dependency .. " is unavailable"
				break
			end
			if dependency_record.status ~= "ready" then
				blocked = "dependency " .. dependency .. " is " .. dependency_record.status
				break
			end
		end
		if blocked then
			record.status, record.detail = "unavailable", blocked
		else
			local cancelled = false
			if opts.cancel then
				local ok, value = pcall(opts.cancel, entry.name)
				if not ok or type(value) ~= "boolean" then
					record.status = "failed"
					record.detail = redact.text(ok and "cancellation callback must return boolean" or tostring(value))
				else
					cancelled = value
				end
			end
			if not record.status and cancelled then
				record.status, record.detail = "cancelled", "health evaluation was cancelled"
			end
			if not record.status then
				local ok, value = xpcall(entry.check, debug.traceback)
				local checked, detail
				if ok then
					checked, detail = status(value)
				else
					detail = redact.text(tostring(value))
				end
				if checked then
					record.status, record.detail = checked.status, checked.detail
				else
					record.status, record.detail = "failed", detail or "health check returned an invalid status"
				end
			end
		end
		results[record.name] = record
		table.insert(result, record)
	end
	return { components = vim.deepcopy(result), by_name = vim.deepcopy(results) }
end

return M
