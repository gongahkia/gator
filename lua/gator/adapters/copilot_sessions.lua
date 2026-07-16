local M = {}

local function fail(message)
	error("Gator Copilot sessions: " .. message, 3)
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

local function session(result)
	return { provider = "copilot", id = id(result.sessionId, "sessionId"), owner = "provider" }
end

function M.create(opts)
	if type(opts) ~= "table" then
		fail("create requires request and cwd")
	end
	return session(request(client(opts.request), "session/new", { cwd = cwd(opts.cwd), mcpServers = {} }))
end

function M.list()
	return { available = false, reason = "Copilot ACP does not advertise a documented session-list contract" }
end

function M.resume(opts)
	if type(opts) ~= "table" then
		fail("resume requires request, id, and cwd")
	end
	return session(request(client(opts.request), "session/load", {
		sessionId = id(opts.id, "id"),
		cwd = cwd(opts.cwd),
		mcpServers = {},
	}))
end

function M.close()
	return { available = false, reason = "Copilot ACP does not advertise a documented session-close contract" }
end

return M
