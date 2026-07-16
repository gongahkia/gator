local domains = {
	task = true,
	session = true,
	context = true,
	run = true,
	workspace = true,
	review = true,
	policy = true,
}
local M = {
	api_version = 1,
	schema_version = 1,
	domains = vim.deepcopy(domains),
}
local listeners, order, sequence = {}, {}, 0

local function fail(message)
	error("Gator events: " .. message, 3)
end

local function fields(value, allowed, name)
	if type(value) ~= "table" then
		fail(name .. " must be a table")
	end
	for key in pairs(value) do
		if not allowed[key] then
			fail(name .. " contains unsupported field: " .. tostring(key))
		end
	end
end

local function event_type(value, name)
	if type(value) ~= "string" then
		fail(name .. " must be an event type")
	end
	local domain, action = value:match("^([a-z][a-z0-9_-]*)%.([a-z][a-z0-9_-]*)$")
	if not domain or not action or not domains[domain] then
		fail(name .. " has an unsupported domain")
	end
	return value
end

local function timestamp(value, name)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail(name .. " must be a non-negative integer timestamp")
	end
	return value
end

local function payload(value, path)
	if type(value) == "string" or type(value) == "number" or type(value) == "boolean" then
		return value
	end
	if type(value) ~= "table" then
		fail(path .. " must be JSON-compatible")
	end
	local result = {}
	if vim.islist(value) then
		for index, item in ipairs(value) do
			result[index] = payload(item, path .. "[" .. index .. "]")
		end
		return result
	end
	for key, item in pairs(value) do
		if type(key) ~= "string" then
			fail(path .. " keys must be strings")
		end
		local lower = key:lower()
		if lower:match("token") or lower:match("secret") or lower:match("credential") or lower:match("password") then
			fail(path .. " must not include credentials")
		end
		result[key] = payload(item, path .. "." .. key)
	end
	if result.owner ~= nil then
		if
			type(result.provider) ~= "string"
			or result.provider == ""
			or type(result.id) ~= "string"
			or result.id == ""
			or result.owner ~= "provider"
		then
			fail(path .. " session references must remain provider-owned")
		end
	end
	return result
end

function M.event(attrs)
	fields(attrs, { schema_version = true, type = true, at = true, payload = true }, "event")
	if attrs.schema_version ~= M.schema_version then
		fail("event schema_version is unsupported")
	end
	return {
		schema_version = M.schema_version,
		type = event_type(attrs.type, "event type"),
		at = timestamp(attrs.at, "event at"),
		payload = payload(attrs.payload == nil and {} or attrs.payload, "event payload"),
	}
end

function M.subscribe(opts)
	fields(opts, { types = true, handler = true }, "subscription")
	if type(opts.handler) ~= "function" then
		fail("subscription handler must be a function")
	end
	local types
	if opts.types ~= nil then
		if type(opts.types) ~= "table" or not vim.islist(opts.types) or #opts.types == 0 then
			fail("subscription types must be a non-empty list")
		end
		types = {}
		for index, value in ipairs(opts.types) do
			value = event_type(value, "subscription types[" .. index .. "]")
			if types[value] then
				fail("subscription types must not contain duplicates")
			end
			types[value] = true
		end
	end
	sequence = sequence + 1
	local id = "event-listener-" .. sequence
	listeners[id] = { types = types, handler = opts.handler }
	table.insert(order, id)
	return id
end

function M.unsubscribe(id)
	if type(id) ~= "string" or not id:match("^event%-listener%-[1-9]%d*$") then
		fail("listener id is invalid")
	end
	if not listeners[id] then
		return false
	end
	listeners[id] = nil
	return true
end

function M.emit(attrs)
	local value = M.event(attrs)
	for _, id in ipairs(order) do
		local listener = listeners[id]
		if listener and (not listener.types or listener.types[value.type]) then
			local ok = pcall(listener.handler, vim.deepcopy(value))
			if not ok then
				fail("listener " .. id .. " failed")
			end
		end
	end
	return value
end

return M
