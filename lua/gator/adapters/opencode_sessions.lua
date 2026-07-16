local M = {}

local function fail(message)
	error("Gator OpenCode sessions: " .. message, 3)
end

local function client(value)
	if type(value) ~= "function" then
		fail("request must be a function")
	end
	return value
end

local function id(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty opaque session id")
	end
	return value
end

local function cwd(value)
	if type(value) ~= "string" or value == "" then
		fail("cwd must be a non-empty string")
	end
	return value
end

local function request(callback, method, params)
	local ok, result = pcall(callback, method, params)
	if not ok or type(result) ~= "table" then
		fail(method .. " request failed")
	end
	if result.error ~= nil then
		fail(method .. " request returned an error")
	end
	return result
end

local function session(value, name)
	return { provider = "opencode", id = id(value, name), owner = "provider" }
end

function M.create(opts)
	if type(opts) ~= "table" then
		fail("create requires request and cwd")
	end
	for key in pairs(opts) do
		if key ~= "request" and key ~= "cwd" then
			fail("create contains unsupported field: " .. tostring(key))
		end
	end
	local result = request(client(opts.request), "session/new", { cwd = cwd(opts.cwd), mcpServers = {} })
	return session(result.sessionId, "sessionId")
end

function M.list(opts)
	if type(opts) ~= "table" then
		fail("list requires request")
	end
	for key in pairs(opts) do
		if key ~= "request" and key ~= "cwd" then
			fail("list contains unsupported field: " .. tostring(key))
		end
	end
	local params = {}
	if opts.cwd ~= nil then
		params.cwd = cwd(opts.cwd)
	end
	local result = request(client(opts.request), "session/list", params)
	if type(result.sessions) ~= "table" or not vim.islist(result.sessions) then
		fail("session/list returned unsupported data")
	end
	local sessions = {}
	for index, value in ipairs(result.sessions) do
		sessions[index] = session(type(value) == "table" and value.sessionId, "session " .. index .. " id")
	end
	return sessions
end

function M.resume(opts)
	if type(opts) ~= "table" then
		fail("resume requires request, id, and cwd")
	end
	for key in pairs(opts) do
		if key ~= "request" and key ~= "id" and key ~= "cwd" then
			fail("resume contains unsupported field: " .. tostring(key))
		end
	end
	local session_id = id(opts.id, "id")
	request(client(opts.request), "session/resume", {
		sessionId = session_id,
		cwd = cwd(opts.cwd),
		mcpServers = {},
	})
	return session(session_id, "id")
end

function M.close(opts)
	if type(opts) ~= "table" then
		fail("close requires request and id")
	end
	for key in pairs(opts) do
		if key ~= "request" and key ~= "id" then
			fail("close contains unsupported field: " .. tostring(key))
		end
	end
	request(client(opts.request), "session/close", { sessionId = id(opts.id, "id") })
	return true
end

return M
