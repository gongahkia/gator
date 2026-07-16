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
