local stream = require("gator.adapters.cline_stream")
local M = {}

local function fail(message)
	error("Gator Cline sessions: " .. message, 3)
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
	if type(callback) ~= "function" then
		fail("request must be a function")
	end
	local ok, result = pcall(callback, method, params)
	if not ok or type(result) ~= "table" or result.error ~= nil then
		fail(method .. " request failed")
	end
	return result
end

local function session(value)
	return { provider = "cline", id = id(value, "sessionId"), owner = "provider" }
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
	return session(request(opts.request, "session/new", { cwd = cwd(opts.cwd), mcpServers = {} }).sessionId)
end

function M.list()
	return { available = false, reason = "Cline history does not document a machine-readable session-list schema" }
end

function M.resume(opts)
	if type(opts) ~= "table" or type(opts.run) ~= "function" or type(opts.prompt) ~= "string" or opts.prompt == "" then
		fail("resume requires run and prompt")
	end
	for key in pairs(opts) do
		if key ~= "run" and key ~= "id" and key ~= "prompt" and key ~= "cwd" and key ~= "executable" then
			fail("resume contains unsupported field: " .. tostring(key))
		end
	end
	local session_id = id(opts.id, "id")
	local argv = { opts.executable or "cline", "--id", session_id, "--json" }
	if opts.cwd ~= nil then
		table.insert(argv, "--cwd")
		table.insert(argv, cwd(opts.cwd))
	end
	table.insert(argv, opts.prompt)
	local ok, result = pcall(opts.run, argv)
	if not ok or type(result) ~= "table" or result.code ~= 0 or type(result.stdout) ~= "string" then
		fail("Cline session resume failed")
	end
	stream.parse(result.stdout)
	return { provider = "cline", id = session_id, owner = "provider" }
end

function M.close()
	return { available = false, reason = "Cline does not document provider-owned session deletion or close" }
end

return M
