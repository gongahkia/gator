if vim.env.GATOR_LIVE_CODEX ~= "1" then
	return
end

local login = vim.system({ "codex", "login", "status" }, { text = true }):wait()
assert(login.code == 0, "protected Codex verification requires an existing CLI-owned login")
local app_server = vim.system({ "codex", "app-server", "--help" }, { text = true }):wait()
assert(
	app_server.code == 0 and (app_server.stdout or ""):find("stdio://", 1, true),
	"protected Codex verification requires the app-server stdio capability"
)

if vim.env.GATOR_LIVE_CODEX_AUTH ~= "1" then
	return
end

local redact = require("gator.policy.redact")
local uv = vim.uv
local stdin, stdout, stderr = uv.new_pipe(false), uv.new_pipe(false), uv.new_pipe(false)
local responses, turns, completed, buffer = {}, {}, {}, ""
local handle
local function close()
	if handle and not handle:is_closing() then
		handle:kill("sigterm")
	end
	for _, pipe in ipairs({ stdin, stdout, stderr }) do
		if pipe and not pipe:is_closing() then
			pipe:close()
		end
	end
end
local function record(data)
	buffer = buffer .. data
	while true do
		local ending = buffer:find("\n", 1, true)
		if not ending then
			return
		end
		local line = buffer:sub(1, ending - 1)
		buffer = buffer:sub(ending + 1)
		local ok, value = pcall(vim.json.decode, line)
		if ok and type(value) == "table" then
			if value.id ~= nil then
				responses[value.id] = value
			elseif
				value.method == "turn/started"
				and type(value.params) == "table"
				and type(value.params.threadId) == "string"
			then
				local turn = value.params.turn
				if type(turn) == "table" and type(turn.id) == "string" then
					turns[value.params.threadId] = turn.id
				end
			elseif
				value.method == "turn/completed"
				and type(value.params) == "table"
				and type(value.params.threadId) == "string"
			then
				local turn = value.params.turn
				if type(turn) == "table" and type(turn.id) == "string" and type(turn.status) == "string" then
					completed[value.params.threadId] = { id = turn.id, status = turn.status }
				end
			end
		end
	end
end
local function request(id, method, params)
	stdin:write(vim.json.encode({ id = id, method = method, params = params }) .. "\n")
	local function response()
		return responses[id]
	end
	assert(
		vim.wait(10000, function()
			return response() ~= nil
		end),
		"authenticated Codex verification timed out waiting for " .. method
	)
	local value = response()
	if type(value.result) ~= "table" then
		local detail = type(value.error) == "table" and redact.text(tostring(value.error.message))
			or "unknown native failure"
		error("authenticated Codex verification requires a successful " .. method .. ": " .. detail)
	end
	return value.result
end
local workspace = vim.fn.tempname()
assert(vim.fn.mkdir(workspace, "p") == 1, "authenticated Codex verification requires a temporary workspace")
local thread_id
local ok, failure = xpcall(function()
	handle = assert(
		uv.spawn("codex", { args = { "app-server", "--stdio" }, stdio = { stdin, stdout, stderr } }, function() end)
	)
	stdout:read_start(function(error, data)
		assert(not error, "authenticated Codex verification received app-server stdout failure")
		if data then
			record(data)
		end
	end)
	request("gator-live-initialize", "initialize", { clientInfo = { name = "gator", version = "1" } })
	local started = request("gator-live-thread-start", "thread/start", { cwd = workspace, ephemeral = false })
	assert(
		type(started.thread) == "table" and type(started.thread.id) == "string" and started.thread.id ~= "",
		"authenticated Codex verification requires a native recoverable thread"
	)
	thread_id = started.thread.id
	stdin:write(vim.json.encode({
		id = "gator-live-turn-start",
		method = "turn/start",
		params = {
			threadId = thread_id,
			input = { { type = "text", text = "Do not use tools or edit files. Wait until interrupted." } },
		},
	}) .. "\n")
	assert(
		vim.wait(10000, function()
			return turns[thread_id] ~= nil
		end),
		"authenticated Codex verification requires a native active turn notification"
	)
	request("gator-live-turn-interrupt", "turn/interrupt", { threadId = thread_id, turnId = turns[thread_id] })
	assert(
		vim.wait(10000, function()
			return completed[thread_id] and completed[thread_id].id == turns[thread_id]
		end),
		"authenticated Codex verification requires interrupted turn completion"
	)
	assert(
		completed[thread_id].status == "interrupted",
		"authenticated Codex verification requires native interrupted turn status"
	)
	local resumed = request("gator-live-thread-resume", "thread/resume", { threadId = thread_id })
	assert(
		type(resumed.thread) == "table" and resumed.thread.id == thread_id,
		"authenticated Codex verification must preserve native thread ownership through interruption and recovery"
	)
end, debug.traceback)
if thread_id then
	local deleted, delete_failure =
		pcall(request, "gator-live-thread-delete", "thread/delete", { threadId = thread_id })
	if not deleted and ok then
		ok, failure = false, delete_failure
	end
end
close()
vim.fn.delete(workspace, "d")
assert(ok, failure)
