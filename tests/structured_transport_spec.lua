local structured = require("gator.adapters.structured")

local function fake_manager()
	local sent, process = {}, {}
	local manager = structured.new({
		spawn = function(argv, opts, done)
			process.argv, process.stdout, process.done = argv, opts.stdout, done
			return {
				write = function(_, value)
					table.insert(sent, vim.json.decode(vim.trim(value)))
					return true
				end,
				kill = function()
					return true
				end,
			}
		end,
	})
	return manager, sent, process
end

local manager, sent, process = fake_manager()
local session, events, usage = nil, {}, nil
local pi = manager:open({
	provider = "pi",
	cwd = vim.fn.getcwd(),
	prompt = "fix this",
	on_session = function(value)
		session = value
	end,
	on_event = function(kind, value)
		table.insert(events, { kind = kind, value = value })
	end,
	on_usage = function(value)
		usage = value
	end,
})
assert(
	process.argv[1] == "pi" and process.argv[2] == "--mode" and sent[1].type == "get_state",
	"Pi chat must use documented RPC mode"
)
process.stdout(
	nil,
	vim.json.encode({ type = "response", command = "get_state", success = true, data = { sessionId = "pi-1" } }) .. "\n"
)
assert(
	session.id == "pi-1" and sent[2].type == "prompt" and sent[2].message == "fix this",
	"Pi state must establish a provider session before prompting"
)
process.stdout(nil, vim.json.encode({ type = "agent_start" }) .. "\n")
process.stdout(
	nil,
	vim.json.encode({ type = "message_update", assistantMessageEvent = { type = "text_delta", delta = "hello" } })
		.. "\n"
)
process.stdout(nil, vim.json.encode({
	type = "response",
	command = "get_session_stats",
	success = true,
	data = { tokens = { input = 4, output = 3, total = 7 } },
}) .. "\n")
process.stdout(nil, vim.json.encode({ type = "agent_settled" }) .. "\n")
assert(
	events[1].kind == "running" and events[2].value == "hello" and usage.total_tokens == 7,
	"Pi events and reported usage must flow to the Gator chat"
)
assert(pi.cancel() and sent[#sent].type == "abort", "Pi chat cancellation must use the RPC abort command")

manager, sent, process = fake_manager()
local resumed_pi = nil
manager:resume({
	provider = "pi",
	cwd = vim.fn.getcwd(),
	session = { id = "pi-existing", path = "/tmp/pi-existing.jsonl" },
	on_session = function(value)
		resumed_pi = value
	end,
})
assert(
	vim.deep_equal(process.argv, { "pi", "--mode", "rpc", "--session", "/tmp/pi-existing.jsonl" })
		and sent[1].type == "get_state",
	"Pi chat resume must reopen the documented persisted session without fabricating a prompt"
)
process.stdout(nil, vim.json.encode({
	type = "response",
	command = "get_state",
	success = true,
	data = { sessionId = "pi-existing", sessionFile = "/tmp/pi-existing.jsonl" },
}) .. "\n")
assert(
	resumed_pi.id == "pi-existing" and resumed_pi.path == "/tmp/pi-existing.jsonl" and resumed_pi.resume_supported,
	"Pi resume must retain the durable session identity and path"
)

manager, sent, process = fake_manager()
local rejected_pi = nil
manager:resume({
	provider = "pi",
	cwd = vim.fn.getcwd(),
	session = { id = "pi-existing" },
	on_session = function()
		rejected_pi = "accepted"
	end,
	on_event = function(kind, value)
		rejected_pi = kind == "error" and value or rejected_pi
	end,
})
process.stdout(nil, vim.json.encode({
	type = "response",
	command = "get_state",
	success = true,
	data = { sessionId = "pi-other" },
}) .. "\n")
assert(
	rejected_pi == "provider resume returned a different session identity",
	"Pi resume must reject a provider response that does not confirm the stored session"
)

manager, sent, process = fake_manager()
manager:fork({ provider = "pi", cwd = vim.fn.getcwd(), session = { id = "pi-existing" }, prompt = "continue" })
assert(
	vim.deep_equal(process.argv, { "pi", "--mode", "rpc", "--fork", "pi-existing" }),
	"Pi native forks must use Pi's documented session fork flag"
)

manager, sent, process = fake_manager()
local codex_session, codex_events = nil, {}
manager:open({
	provider = "codex",
	cwd = vim.fn.getcwd(),
	prompt = "review this",
	on_session = function(value)
		codex_session = value
	end,
	on_event = function(kind, value)
		table.insert(codex_events, { kind = kind, value = value })
	end,
})
assert(
	process.argv[1] == "codex" and process.argv[2] == "app-server" and sent[1].method == "initialize",
	"Codex chat must start App Server"
)
process.stdout(nil, vim.json.encode({ jsonrpc = "2.0", id = 1, result = {} }) .. "\n")
assert(
	sent[2].method == "initialized" and sent[3].method == "thread/start",
	"Codex initialization must complete before thread creation"
)
process.stdout(nil, vim.json.encode({ jsonrpc = "2.0", id = 2, result = { thread = { id = "thread-1" } } }) .. "\n")
assert(
	codex_session.id == "thread-1" and sent[4].method == "turn/start",
	"Codex thread creation must precede the first turn"
)
process.stdout(
	nil,
	vim.json.encode({ jsonrpc = "2.0", method = "item/agentMessage/delta", params = { delta = "done" } }) .. "\n"
)
process.stdout(nil, vim.json.encode({ jsonrpc = "2.0", method = "turn/completed", params = {} }) .. "\n")
assert(
	codex_events[1].value == "done" and codex_events[2].kind == "settled",
	"Codex deltas and turn completion must update chat state"
)

manager, sent, process = fake_manager()
manager:open({
	provider = "codex",
	cwd = vim.fn.getcwd(),
	prompt = "review this",
	codex_policy = { sandbox = "readOnly", approval_policy = "on-request" },
})
process.stdout(nil, vim.json.encode({ jsonrpc = "2.0", id = 1, result = {} }) .. "\n")
assert(
	sent[3].method == "thread/start"
		and sent[3].params.sandbox == "readOnly"
		and sent[3].params.approvalPolicy == "on-request",
	"Codex structured launches must pass the recorded sandbox and approval policy to App Server"
)
assert(not pcall(manager.open, manager, {
	provider = "pi",
	cwd = vim.fn.getcwd(),
	prompt = "review this",
	codex_policy = { sandbox = "readOnly", approval_policy = "on-request" },
}), "Codex launch policy must not be silently applied to a different provider")

manager, sent, process = fake_manager()
local resumed_codex = nil
manager:resume({
	provider = "codex",
	cwd = vim.fn.getcwd(),
	session = { id = "thread-existing" },
	on_session = function(value)
		resumed_codex = value
	end,
})
process.stdout(nil, vim.json.encode({ jsonrpc = "2.0", id = 1, result = {} }) .. "\n")
assert(
	sent[3].method == "thread/resume" and sent[3].params.threadId == "thread-existing",
	"Codex chat resume must use the documented App Server thread resume request"
)
process.stdout(
	nil,
	vim.json.encode({ jsonrpc = "2.0", id = 2, result = { thread = { id = "thread-existing" } } }) .. "\n"
)
assert(
	resumed_codex.id == "thread-existing" and resumed_codex.resume_supported and #sent == 3,
	"Codex resume must reopen history without creating a synthetic turn"
)

manager, sent, process = fake_manager()
local rejected_codex = nil
manager:resume({
	provider = "codex",
	cwd = vim.fn.getcwd(),
	session = { id = "thread-existing" },
	on_session = function()
		rejected_codex = "accepted"
	end,
	on_event = function(kind, value)
		rejected_codex = kind == "error" and value or rejected_codex
	end,
})
process.stdout(nil, vim.json.encode({ jsonrpc = "2.0", id = 1, result = {} }) .. "\n")
process.stdout(nil, vim.json.encode({ jsonrpc = "2.0", id = 2, result = { thread = { id = "thread-other" } } }) .. "\n")
assert(
	rejected_codex == "provider resume returned a different session identity",
	"Codex resume must reject a provider response that does not confirm the stored thread"
)

manager, sent, process = fake_manager()
manager:fork({ provider = "codex", cwd = vim.fn.getcwd(), session = { id = "thread-existing" }, prompt = "continue" })
process.stdout(nil, vim.json.encode({ jsonrpc = "2.0", id = 1, result = {} }) .. "\n")
assert(
	sent[3].method == "thread/fork" and sent[3].params.threadId == "thread-existing",
	"Codex native forks must use the App Server thread fork request"
)

manager, sent, process = fake_manager()
local approval = nil
manager:open({
	provider = "codex",
	run_id = "run-codex",
	cwd = vim.fn.getcwd(),
	prompt = "change this",
	on_approval = function(request, decide)
		approval = request
		decide("approved")
	end,
})
process.stdout(nil, vim.json.encode({ jsonrpc = "2.0", id = 1, result = {} }) .. "\n")
process.stdout(
	nil,
	vim.json.encode({ jsonrpc = "2.0", id = 2, result = { thread = { id = "thread-approval" } } }) .. "\n"
)
process.stdout(nil, vim.json.encode({
	jsonrpc = "2.0",
	id = "approval-1",
	method = "item/commandExecution/requestApproval",
	params = {
		threadId = "thread-approval",
		turnId = "turn-approval",
		itemId = "item-approval",
		startedAtMs = 1,
		command = "git status",
	},
}) .. "\n")
assert(
	approval.action == "execute command"
		and approval.command == "git status"
		and sent[#sent].id == "approval-1"
		and sent[#sent].result.decision == "accept",
	"Codex approval requests must be surfaced and receive an explicit user decision"
)
