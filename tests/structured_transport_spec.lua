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
assert(process.argv[1] == "pi" and process.argv[2] == "--mode" and sent[1].type == "get_state", "Pi chat must use documented RPC mode")
process.stdout(nil, vim.json.encode({ type = "response", command = "get_state", success = true, data = { sessionId = "pi-1" } }) .. "\n")
assert(session.id == "pi-1" and sent[2].type == "prompt" and sent[2].message == "fix this", "Pi state must establish a provider session before prompting")
process.stdout(nil, vim.json.encode({ type = "agent_start" }) .. "\n")
process.stdout(nil, vim.json.encode({ type = "message_update", assistantMessageEvent = { type = "text_delta", delta = "hello" } }) .. "\n")
process.stdout(nil, vim.json.encode({ type = "response", command = "get_session_stats", success = true, data = { tokens = { input = 4, output = 3, total = 7 } } }) .. "\n")
process.stdout(nil, vim.json.encode({ type = "agent_settled" }) .. "\n")
assert(events[1].kind == "running" and events[2].value == "hello" and usage.total_tokens == 7, "Pi events and reported usage must flow to the Gator chat")
assert(pi.cancel() and sent[#sent].type == "abort", "Pi chat cancellation must use the RPC abort command")

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
assert(process.argv[1] == "codex" and process.argv[2] == "app-server" and sent[1].method == "initialize", "Codex chat must start App Server")
process.stdout(nil, vim.json.encode({ jsonrpc = "2.0", id = 1, result = {} }) .. "\n")
assert(sent[2].method == "initialized" and sent[3].method == "thread/start", "Codex initialization must complete before thread creation")
process.stdout(nil, vim.json.encode({ jsonrpc = "2.0", id = 2, result = { thread = { id = "thread-1" } } }) .. "\n")
assert(codex_session.id == "thread-1" and sent[4].method == "turn/start", "Codex thread creation must precede the first turn")
process.stdout(nil, vim.json.encode({ jsonrpc = "2.0", method = "item/agentMessage/delta", params = { delta = "done" } }) .. "\n")
process.stdout(nil, vim.json.encode({ jsonrpc = "2.0", method = "turn/completed", params = {} }) .. "\n")
assert(codex_events[1].value == "done" and codex_events[2].kind == "settled", "Codex deltas and turn completion must update chat state")
