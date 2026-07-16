local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local fixtures = require("gator").module("adapters").fixtures
local root = vim.g.gator_test.root .. "/tests/fixtures/adapters"

assert(vim.fn.isdirectory(root) == 1, "adapter fixture directory must exist")

local jsonl = {}
assert(fixtures.replay_jsonl(root .. "/stream.jsonl", function(record, index)
	jsonl[index] = record.type
end) == 2, "JSONL replay must return record count")
assert(vim.deep_equal(jsonl, { "message", "complete" }), "JSONL replay must preserve record order")

local jsonrpc = {}
assert(fixtures.replay_jsonrpc(root .. "/rpc.jsonl", function(message, index)
	jsonrpc[index] = message
end) == 3, "JSON-RPC replay must return record count")
assert(jsonrpc[1].method == "session/update", "JSON-RPC replay must preserve notifications")
assert(jsonrpc[2].result.session == "native-1", "JSON-RPC replay must preserve results")
assert(jsonrpc[3].error.code == -32601, "JSON-RPC replay must preserve errors")

local codex = {}
assert(fixtures.replay_jsonl(root .. "/codex_appserver.jsonl", function(record)
	table.insert(codex, record)
end) == 4, "Codex app-server fixture must replay every record")
assert(
	codex[3].method == "account/read" and codex[4].result.account.type == "chatgpt",
	"Codex fixture must preserve authenticated app-server traffic"
)

local claude = {}
assert(fixtures.replay_jsonl(root .. "/claude_stream.jsonl", function(record)
	table.insert(claude, record)
end) == 3, "Claude stream fixture must replay every record")
assert(
	claude[1].session_id == "claude-fixture" and claude[3].subtype == "success",
	"Claude fixture must preserve structured session and completion data"
)

local gemini = {}
assert(fixtures.replay_jsonl(root .. "/gemini_stream.jsonl", function(record)
	table.insert(gemini, record)
end) == 6, "Gemini stream fixture must replay every record")
assert(
	gemini[1].session_id == "gemini-fixture"
		and gemini[3].tool_name == "list_directory"
		and gemini[6].status == "success",
	"Gemini fixture must preserve session, tool, and completion data"
)

local copilot = {}
assert(fixtures.replay_jsonrpc(root .. "/copilot_acp.jsonl", function(message)
	table.insert(copilot, message)
end) == 5, "Copilot ACP fixture must replay every record")
assert(
	copilot[2].result.agentCapabilities.loadSession == false
		and copilot[4].result.sessionId == "copilot-fixture"
		and copilot[5].params.update.availableCommands[1].name == "context",
	"Copilot fixture must preserve ACP capability, session, and command updates"
)

local opencode = {}
assert(fixtures.replay_jsonrpc(root .. "/opencode_acp.jsonl", function(message)
	table.insert(opencode, message)
end) == 5, "OpenCode ACP fixture must replay every record")
assert(
	opencode[2].result.agentCapabilities.loadSession
		and opencode[2].result.agentCapabilities.sessionCapabilities.resume ~= nil
		and opencode[4].result.sessionId == "opencode-fixture"
		and opencode[5].params.update.availableCommands[1].name == "help",
	"OpenCode fixture must preserve ACP capabilities, sessions, and command updates"
)

local terminal = {}
assert(fixtures.replay_terminal(root .. "/terminal.json", function(chunk, index)
	terminal[index] = chunk
end) == 3, "terminal replay must return chunk count")
assert(table.concat(terminal) == "\27[?25lworking\rdone\n", "terminal replay must preserve control sequences")

local process = { stdout = {}, stderr = {} }
local status = fixtures.replay_process(root .. "/process.json", {
	stdout = function(chunk)
		table.insert(process.stdout, chunk)
	end,
	stderr = function(chunk)
		table.insert(process.stderr, chunk)
	end,
	exit = function(result)
		process.exit = result.code
	end,
})
assert(status.code == 7 and process.exit == 7, "process replay must expose non-zero exits")
assert(table.concat(process.stdout) == "out-1\n", "process replay must preserve stdout")
assert(table.concat(process.stderr) == "failure\n", "process replay must preserve stderr")

local invalid = helpers.tempdir("invalid") .. "/stream.jsonl"
helpers.write(invalid, "not json\n")
local ok = pcall(fixtures.replay_jsonl, invalid, function() end)
assert(not ok, "invalid JSONL fixtures must fail explicitly")
