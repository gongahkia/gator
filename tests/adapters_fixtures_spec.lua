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

local amp = {}
assert(fixtures.replay_jsonl(root .. "/amp_stream.jsonl", function(record)
	table.insert(amp, record)
end) == 3, "Amp stream fixture must replay every record")
assert(
	amp[1].session_id == "T-amp-fixture"
		and amp[2].message.content[1].text == "gator-fixture"
		and amp[3].subtype == "success",
	"Amp fixture must preserve documented stream JSON thread and completion records"
)

local codex = {}
assert(fixtures.replay_jsonl(root .. "/codex_appserver.jsonl", function(record)
	table.insert(codex, record)
end) == 5, "Codex app-server fixture must replay every record")
assert(
	codex[2].result.platformOs == "macos" and codex[4].method == "account/read" and codex[5].result.requiresOpenaiAuth,
	"Codex fixture must preserve sanitized app-server transport traffic"
)

local codex_stream = {}
assert(fixtures.replay_jsonl(root .. "/codex_stream.jsonl", function(record)
	table.insert(codex_stream, record)
end) == 5, "Codex stream fixture must replay every native notification")
assert(
	codex_stream[1].method == "turn/started"
		and codex_stream[3].params.delta == "gator-fixture"
		and codex_stream[5].params.turn.status == "completed",
	"Codex stream fixture must preserve native turn and assistant-message notifications"
)

local codex_permissions = {}
assert(fixtures.replay_jsonl(root .. "/codex_permissions.jsonl", function(record)
	table.insert(codex_permissions, record)
end) == 3, "Codex permission fixture must replay every native request")
assert(
	codex_permissions[1].method == "item/commandExecution/requestApproval"
		and codex_permissions[2].params.itemId == "file-fixture"
		and codex_permissions[3].params.permissions.network.enabled,
	"Codex permission fixture must preserve command, file, and permission approval requests"
)

local codex_signals = {}
assert(fixtures.replay_jsonl(root .. "/codex_signals.jsonl", function(record)
	table.insert(codex_signals, record)
end) == 4, "Codex signal fixture must replay every native notification")
assert(
	codex_signals[1].params.tokenUsage.last.totalTokens == 5
		and codex_signals[2].params.changes[1].kind.type == "update"
		and codex_signals[4].params.item.type == "contextCompaction",
	"Codex signal fixture must preserve usage, patch, and compaction notifications"
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

local cline = {}
assert(fixtures.replay_jsonrpc(root .. "/cline_acp.jsonl", function(message)
	table.insert(cline, message)
end) == 5, "Cline ACP fixture must replay every record")
assert(
	cline[2].result.agentCapabilities.loadSession
		and cline[4].result.sessionId == "cline-fixture"
		and cline[5].params.update.content.text == "gator-fixture",
	"Cline fixture must preserve ACP capability, session, and streamed update records"
)

local cline_stream = {}
assert(fixtures.replay_jsonl(root .. "/cline_stream.jsonl", function(record)
	table.insert(cline_stream, record)
end) == 2, "Cline stream fixture must replay every record")
assert(
	cline_stream[1].type == "say" and cline_stream[2].partial == false,
	"Cline fixture must preserve documented JSON message records"
)

local cursor = {}
assert(fixtures.replay_jsonl(root .. "/cursor_stream.jsonl", function(record)
	table.insert(cursor, record)
end) == 6, "Cursor stream fixture must replay every record")
assert(
	cursor[1].session_id == "cursor-fixture"
		and cursor[5].tool_call.readToolCall.result.success.totalLines == 1
		and cursor[6].subtype == "success",
	"Cursor fixture must preserve documented stream session, tool, and completion records"
)

local droid = {}
assert(fixtures.replay_jsonl(root .. "/droid_result.json", function(record)
	table.insert(droid, record)
end) == 1, "Droid result fixture must replay one structured result")
assert(
	droid[1].type == "result" and droid[1].session_id == "droid-fixture" and droid[1].is_error == false,
	"Droid fixture must preserve documented JSON result fields"
)

local droid_rpc = {}
assert(fixtures.replay_jsonrpc(root .. "/droid_rpc.jsonl", function(message)
	table.insert(droid_rpc, message)
end) == 3, "Droid JSON-RPC fixture must replay every record")
assert(
	droid_rpc[1].method == "droid.session_notification"
		and droid_rpc[2].result.sessionId == "droid-fixture"
		and droid_rpc[3].method == "droid.request_permission",
	"Droid fixture must preserve native JSON-RPC session traffic"
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

local pi = {}
assert(fixtures.replay_jsonl(root .. "/pi_rpc.jsonl", function(record)
	table.insert(pi, record)
end) == 5, "Pi RPC fixture must replay every record")
assert(
	pi[1].command == "get_state"
		and pi[1].data.sessionId == "pi-fixture"
		and pi[2].data.commands[1].source == "prompt"
		and pi[4].message.content[1].text == "gator-fixture"
		and pi[5].type == "agent_end",
	"Pi fixture must preserve RPC state, commands, and streamed responses"
)

local pi_signals = {}
assert(fixtures.replay_jsonl(root .. "/pi_signals.jsonl", function(record)
	table.insert(pi_signals, record)
end) == 2, "Pi signal fixture must replay every record")
assert(
	pi_signals[1].message.usage.totalTokens == 5 and pi_signals[2].result.tokensBefore == 10,
	"Pi signal fixture must preserve usage and compaction records"
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
