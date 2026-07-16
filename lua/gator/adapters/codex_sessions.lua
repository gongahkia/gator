local M = {}

local function fail(message)
	error("Gator Codex sessions: " .. message, 3)
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

function M.create(opts)
	if type(opts) ~= "table" or type(opts.cwd) ~= "string" or opts.cwd == "" then
		fail("create requires request and cwd")
	end
	local result = request(client(opts.request), "thread/start", { cwd = opts.cwd })
	local thread = result.thread
	return { provider = "codex", id = id(thread and thread.id, "thread.id"), owner = "provider" }
end

function M.list(opts)
	if type(opts) ~= "table" then
		fail("list requires request")
	end
	local result = request(client(opts.request), "thread/list", {})
	if type(result.data) ~= "table" or not vim.islist(result.data) then
		fail("thread/list returned unsupported data")
	end
	local sessions = {}
	for index, thread in ipairs(result.data) do
		sessions[index] =
			{ provider = "codex", id = id(thread and thread.id, "thread " .. index .. " id"), owner = "provider" }
	end
	return sessions
end

function M.resume(opts)
	if type(opts) ~= "table" then
		fail("resume requires request and id")
	end
	local thread_id = id(opts.id, "id")
	local result = request(client(opts.request), "thread/resume", { threadId = thread_id })
	local thread = result.thread
	return { provider = "codex", id = id(thread and thread.id, "thread.id"), owner = "provider" }
end

function M.close(opts)
	if type(opts) ~= "table" then
		fail("close requires request and id")
	end
	request(client(opts.request), "thread/unsubscribe", { threadId = id(opts.id, "id") })
	return true
end

return M
