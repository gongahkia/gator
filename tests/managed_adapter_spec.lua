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

local droid_argv
local droid = managed.new({
	spawn = function(argv)
		droid_argv = argv
		return aider_handle
	end,
})
droid:open({ provider = "droid", cwd = root, task_id = "managed-task", prompt = "start" })
assert(
	vim.tbl_contains(droid_argv, "--cwd")
		and vim.tbl_contains(droid_argv, root)
		and vim.tbl_contains(droid_argv, "--output-format")
		and vim.tbl_contains(droid_argv, "json"),
	"Droid must use its documented one-shot JSON execution fallback, not an undocumented JSON-RPC schema"
)
