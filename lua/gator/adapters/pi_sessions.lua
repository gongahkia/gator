local M = {}

local function fail(message)
	error("Gator Pi sessions: " .. message, 3)
end

local function client(value)
	if type(value) ~= "function" then
		fail("request must be a function")
	end
	return value
end

local function path(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty provider session path")
	end
	return value
end

local function request(callback, command)
	local ok, result = pcall(callback, command)
	if not ok or type(result) ~= "table" or result.type ~= "response" or result.command ~= command.type then
		fail(command.type .. " request failed")
	end
	if result.success ~= true then
		fail(command.type .. " request returned an error")
	end
	return result
end

local function session(result)
	local state = result.data
	if type(state) ~= "table" then
		fail("get_state returned unsupported data")
	end
	return { provider = "pi", id = path(state.sessionFile, "data.sessionFile"), owner = "provider" }
end

local function state(callback)
	return session(request(callback, { type = "get_state" }))
end

function M.create(opts)
	if type(opts) ~= "table" then
		fail("create requires request")
	end
	for key in pairs(opts) do
		if key ~= "request" then
			fail("create contains unsupported field: " .. tostring(key))
		end
	end
	local callback = client(opts.request)
	local created = request(callback, { type = "new_session" })
	if type(created.data) ~= "table" or created.data.cancelled ~= false then
		fail("new_session was cancelled or returned unsupported data")
	end
	return state(callback)
end

function M.list()
	return { available = false, reason = "Pi RPC does not advertise a session-list contract" }
end

function M.resume(opts)
	if type(opts) ~= "table" then
		fail("resume requires request and provider session path")
	end
	for key in pairs(opts) do
		if key ~= "request" and key ~= "id" then
			fail("resume contains unsupported field: " .. tostring(key))
		end
	end
	local callback = client(opts.request)
	local resumed = request(callback, { type = "switch_session", sessionPath = path(opts.id, "id") })
	if type(resumed.data) ~= "table" or resumed.data.cancelled ~= false then
		fail("switch_session was cancelled or returned unsupported data")
	end
	return state(callback)
end

function M.close()
	return { available = false, reason = "Pi RPC does not advertise session deletion or close" }
end

return M
