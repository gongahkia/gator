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
				update = { sessionUpdate = "tool_call", title = "unsafe command", rawInput = { token = "secret" } },
			},
		}) .. "\n")
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
	run_id = "managed-run",
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
		and events[2].text == "managed reply",
	"ACP managed runs must initialize, create, prompt, and render provider text"
)
assert(
	events[1].type == "phase" and events[1].phase == "using a tool" and events[2].type == "text",
	"ACP tool updates must expose only a generic safe progress phase"
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
	run_id = "managed-run",
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
	"Aider managed runs must use a run-scoped Gator-owned history file"
)

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
	run_id = "managed-run",
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
	run_id = "managed-run",
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

local configured_stdout, configured_argv, configured_session, listed = nil, nil, nil, nil
local configured_handle = {}
function configured_handle:write(frame)
	local value = vim.json.decode(frame)
	if value.method == "initialize" then
		configured_stdout(nil, vim.json.encode({
			jsonrpc = "2.0",
			id = value.id,
			result = { protocolVersion = 1, agentCapabilities = { loadSession = true, sessionList = true } },
		}) .. "\n")
	elseif value.method == "session/new" then
		configured_stdout(
			nil,
			vim.json.encode({ jsonrpc = "2.0", id = value.id, result = { sessionId = "configured-session" } }) .. "\n"
		)
	elseif value.method == "session/list" then
		configured_stdout(nil, vim.json.encode({
			jsonrpc = "2.0",
			id = value.id,
			result = { sessions = { { sessionId = "configured-session" } } },
		}) .. "\n")
	end
	return true
end
function configured_handle:kill()
	return true
end
local configured = managed.new({
	commands = { localagent = { argv = { "local-agent", "--acp" } } },
	spawn = function(argv, opts)
		configured_argv, configured_stdout = argv, opts.stdout
		return configured_handle
	end,
})
configured:open({
	provider = "localagent",
	cwd = root,
	run_id = "configured-run",
	on_session = function(value)
		configured_session = value
	end,
})
assert(
	vim.deep_equal(configured_argv, { "local-agent", "--acp" })
		and configured_session.capabilities.loadSession
		and configured_session.capabilities.sessionList,
	"explicitly configured ACP commands must launch only through negotiated capability state"
)
assert(
	configured:list(configured_session, function(value)
		listed = value
	end),
	"ACP session/list must be callable only after the agent advertises it"
)
assert(
	listed.sessions[1].sessionId == "configured-session",
	"capability-negotiated ACP session/list must preserve the agent response"
)
