local managed = require("gator.adapters.managed")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local root = helpers.tempdir("managed-adapter")
local writes, sessions, events, decisions = {}, {}, {}, {}
local stdout
local handle = {}

function handle:write(frame)
	local value = vim.json.decode(frame)
	table.insert(writes, value)
	if value.method == "initialize" then
		stdout(nil, vim.json.encode({ jsonrpc = "2.0", id = value.id, result = { protocolVersion = 1 } }) .. "\n")
	elseif value.method == "session/new" then
		stdout(
			nil,
			vim.json.encode({ jsonrpc = "2.0", id = value.id, result = { sessionId = "managed-session" } }) .. "\n"
		)
	elseif value.method == "session/prompt" then
		stdout(nil, vim.json.encode({
			jsonrpc = "2.0",
			method = "session/update",
			params = {
				sessionId = "managed-session",
				update = { sessionUpdate = "agent_message_chunk", content = { type = "text", text = "managed reply" } },
			},
		}) .. "\n")
		stdout(nil, vim.json.encode({
			jsonrpc = "2.0",
			id = 91,
			method = "session/request_permission",
			params = {
				sessionId = "managed-session",
				options = {
					{ optionId = "allow", kind = "allow_once" },
					{ optionId = "deny", kind = "reject_once" },
				},
			},
		}) .. "\n")
	end
	return true
end

function handle:kill()
	return true
end

local runtime = managed.new({
	spawn = function(_, opts)
		stdout = opts.stdout
		return handle
	end,
})
runtime:open({
	provider = "gemini",
	cwd = root,
	task_id = "managed-task",
	prompt = "start",
	on_session = function(value)
		table.insert(sessions, value)
	end,
	on_event = function(value)
		table.insert(events, value)
	end,
	on_permission = function(_, respond)
		table.insert(decisions, respond("approved"))
	end,
})
assert(
	#sessions == 1
		and sessions[1].id == "managed-session"
		and sessions[1].owner == "provider"
		and sessions[1].mode == "acp",
	"ACP managed runs must persist the provider session returned by session/new"
)
assert(
	writes[1].method == "initialize"
		and writes[2].method == "session/new"
		and writes[3].method == "session/prompt"
		and events[1].text == "managed reply",
	"ACP managed runs must initialize, create, prompt, and render provider text"
)
assert(
	decisions[1] and writes[#writes].id == 91 and writes[#writes].result.outcome.optionId == "allow",
	"ACP approval requests must select an explicit provider allow option"
)
assert(runtime:is_active(sessions[1]), "live ACP sessions must remain promptable")
assert(runtime:cancel(sessions[1]), "live ACP sessions must accept cancellation")
assert(
	writes[#writes].method == "session/cancel",
	"ACP cancellation must use the documented session cancellation request"
)

local aider_argv, aider_session
local aider_handle = {
	write = function()
		return true
	end,
	kill = function()
		return true
	end,
}
local aider = managed.new({
	spawn = function(argv)
		aider_argv = argv
		return aider_handle
	end,
})
aider:open({
	provider = "aider",
	cwd = root,
	task_id = "managed-task",
	history = root .. "/aider-history.md",
	prompt = "start",
	on_session = function(value)
		aider_session = value
	end,
})
assert(
	aider_session.owner == "gator"
		and aider_session.id == root .. "/aider-history.md"
		and vim.tbl_contains(aider_argv, "--chat-history-file")
		and vim.tbl_contains(aider_argv, "--message"),
	"Aider managed runs must use a task-scoped Gator-owned history file"
)

local droid_argv, droid_stdout
local droid_writes, droid_sessions, droid_events, droid_answers = {}, {}, {}, {}
local droid_kills = 0
local droid_handle = {}
function droid_handle:write(frame)
	local value = vim.json.decode(frame)
	table.insert(droid_writes, value)
	if value.method == "droid.initialize_session" then
		droid_stdout(
			nil,
			vim.json.encode({ jsonrpc = "2.0", id = value.id, result = { sessionId = "droid-session" } }) .. "\n"
		)
	elseif value.method == "droid.add_user_message" then
		droid_stdout(nil, vim.json.encode({
			jsonrpc = "2.0",
			method = "droid.session_notification",
			params = { notification = { type = "assistant_text_delta", textDelta = "Droid reply" } },
		}) .. "\n")
		droid_stdout(nil, vim.json.encode({
			jsonrpc = "2.0",
			id = "droid-permission",
			method = "droid.request_permission",
			params = { toolUses = {}, options = { { label = "Allow once", value = "proceed_once" } } },
		}) .. "\n")
		droid_stdout(nil, vim.json.encode({
			jsonrpc = "2.0",
			id = "droid-question",
			method = "droid.ask_user",
			params = {
				toolCallId = "tool-question",
				questions = { { index = 0, topic = "choice", question = "Continue?", options = { "yes" } } },
			},
		}) .. "\n")
	elseif value.method == "droid.close_session" then
		droid_stdout(nil, vim.json.encode({ jsonrpc = "2.0", id = value.id, result = {} }) .. "\n")
	end
	return true
end
function droid_handle:kill()
	droid_kills = droid_kills + 1
	return true
end
local droid = managed.new({
	spawn = function(argv, opts)
		droid_argv = argv
		droid_stdout = opts.stdout
		return droid_handle
	end,
})
local droid_run = droid:open({
	provider = "droid",
	cwd = root,
	task_id = "managed-task",
	prompt = "start",
	on_session = function(value)
		table.insert(droid_sessions, value)
	end,
	on_event = function(value)
		table.insert(droid_events, value)
	end,
	on_permission = function(_, respond)
		respond("approved")
	end,
	on_question = function(_, respond)
		table.insert(
			droid_answers,
			respond({
				cancelled = false,
				answers = { { index = 0, question = "Continue?", answer = "yes" } },
			})
		)
	end,
})
assert(droid_run, "Droid managed run must start")
assert(
	vim.deep_equal(droid_argv, {
		"droid",
		"exec",
		"--cwd",
		root,
		"--input-format",
		"stream-jsonrpc",
		"--output-format",
		"stream-jsonrpc",
	})
		and droid_sessions[1].id == "droid-session"
		and droid_sessions[1].mode == "droid"
		and droid_writes[1].method == "droid.initialize_session"
		and droid_writes[1].params.autonomyLevel == "off"
		and droid_writes[2].method == "droid.add_user_message"
		and droid_events[1].text == "Droid reply",
	"Droid must create a persistent strict-autonomy JSON-RPC session and stream output"
)
assert(
	droid_writes[3].result.selectedOption == "proceed_once"
		and droid_answers[1]
		and droid_writes[4].result.answers[1].answer == "yes",
	"Droid permissions and questions must receive Gator-owned one-shot answers"
)
assert(
	droid:cancel(droid_sessions[1]) and droid_writes[5].method == "droid.interrupt_session",
	"Droid cancellation must interrupt the active session"
)
droid_stdout(
	nil,
	[[{"jsonrpc":"2.0","id":99,"method":"droid.session_notification","params":{"notification":{"type":"assistant_text_delta"}}}]]
		.. "\n"
)
droid_stdout(nil, [[{"jsonrpc":"2.0","id":100,"result":{}}]] .. "\n")
assert(
	droid_events[#droid_events - 1].text == "Droid emitted invalid JSON-RPC"
		and droid_events[#droid_events].text == "Droid returned an unknown response id",
	"Droid managed runs must reject malformed envelopes and uncorrelated responses"
)
assert(
	droid:stop(droid_sessions[1])
		and droid_writes[6].method == "droid.close_session"
		and droid_writes[6].params.reason == "other"
		and droid_kills == 1
		and not droid:is_active(droid_sessions[1]),
	"Droid stop must close the provider session then terminate the managed child"
)

local timeout_stdout, timeout_session, timeout_kills = nil, nil, 0
local timeout_handle = {}
function timeout_handle:write(frame)
	local value = vim.json.decode(frame)
	if value.method == "droid.initialize_session" then
		timeout_stdout(
			nil,
			vim.json.encode({ jsonrpc = "2.0", id = value.id, result = { sessionId = "timeout-session" } }) .. "\n"
		)
	end
	return true
end
function timeout_handle:kill()
	timeout_kills = timeout_kills + 1
	return true
end
local timeout_runtime = managed.new({
	stop_timeout_ms = 1,
	spawn = function(_, opts)
		timeout_stdout = opts.stdout
		return timeout_handle
	end,
})
timeout_runtime:open({
	provider = "droid",
	cwd = root,
	task_id = "managed-timeout",
	on_session = function(value)
		timeout_session = value
	end,
})
assert(timeout_session and timeout_runtime:stop(timeout_session) and vim.wait(100, function()
	return timeout_kills == 1
end, 1), "Droid stop must terminate a child when close_session does not answer within its bound")

local copilot_stdout, copilot_writes = nil, {}
local copilot_handle = {}
function copilot_handle:write(frame)
	local value = vim.json.decode(frame)
	table.insert(copilot_writes, value)
	if value.method == "initialize" then
		copilot_stdout(nil, vim.json.encode({
			jsonrpc = "2.0",
			id = value.id,
			result = { protocolVersion = 1, agentCapabilities = { loadSession = true } },
		}) .. "\n")
	elseif value.method == "session/load" then
		copilot_stdout(
			nil,
			vim.json.encode({ jsonrpc = "2.0", id = value.id, result = { sessionId = "copilot-session" } }) .. "\n"
		)
	end
	return true
end
function copilot_handle:kill()
	return true
end
local copilot = managed.new({
	spawn = function(_, opts)
		copilot_stdout = opts.stdout
		return copilot_handle
	end,
})
copilot:open({
	provider = "copilot",
	cwd = root,
	task_id = "managed-task",
	session = { provider = "copilot", id = "copilot-session", owner = "provider", mode = "acp" },
})
assert(
	copilot_writes[2].method == "session/load" and copilot_writes[2].params.sessionId == "copilot-session",
	"Copilot reattach must dynamically attempt ACP session loading"
)

local fallback_stdout, fallback, fallback_writes = nil, nil, {}
local fallback_handle = {}
function fallback_handle:write(frame)
	local value = vim.json.decode(frame)
	table.insert(fallback_writes, value)
	if value.method == "initialize" then
		fallback_stdout(
			nil,
			vim.json.encode({ jsonrpc = "2.0", id = value.id, result = { protocolVersion = 1, agentCapabilities = {} } })
				.. "\n"
		)
	end
	return true
end
function fallback_handle:kill()
	return true
end
local fallback_runtime = managed.new({
	spawn = function(_, opts)
		fallback_stdout = opts.stdout
		return fallback_handle
	end,
})
local fallback_run = fallback_runtime:open({
	provider = "copilot",
	cwd = root,
	task_id = "managed-task",
	session = { provider = "copilot", id = "copilot-session", owner = "provider", mode = "acp" },
	on_resume_fallback = function(value)
		fallback = value
	end,
})
assert(
	fallback_run.fallback
		and fallback
		and fallback.session.id == "copilot-session"
		and fallback.reason == "Copilot ACP did not advertise session/load"
		and #fallback_writes == 1,
	"Copilot must use terminal resume fallback without sending unsupported ACP session/load"
)
