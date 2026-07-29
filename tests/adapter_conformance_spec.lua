local fixtures = require("gator").module("adapters").fixtures
local root = vim.g.gator_test.root .. "/tests/fixtures/adapters"

local results = fixtures.conform({
	cases = {
		{
			name = "aider",
			kind = "terminal",
			path = root .. "/aider_terminal.json",
			verify = function(value)
				return value.count == 2 and table.concat(value.records) == "\27[32mAider\27[0m: gator-fixture\n"
			end,
		},
		{
			name = "amp",
			kind = "jsonl",
			path = root .. "/amp_stream.jsonl",
			verify = function(value)
				return value.count == 3 and value.records[1].session_id == "T-amp-fixture"
			end,
		},
		{
			name = "cline",
			kind = "jsonrpc",
			path = root .. "/cline_acp.jsonl",
			verify = function(value)
				return value.count == 5 and value.records[4].result.sessionId == "cline-fixture"
			end,
		},
		{
			name = "cursor",
			kind = "jsonl",
			path = root .. "/cursor_stream.jsonl",
			verify = function(value)
				return value.count == 6 and value.records[1].session_id == "cursor-fixture"
			end,
		},
		{
			name = "codex",
			kind = "jsonl",
			path = root .. "/codex_appserver.jsonl",
			verify = function(value)
				return value.count == 5 and value.records[2].result.platformOs == "macos"
			end,
		},
		{
			name = "gemini",
			kind = "jsonl",
			path = root .. "/gemini_stream.jsonl",
			verify = function(value)
				return value.count == 6 and value.records[1].session_id == "gemini-fixture"
			end,
		},
		{
			name = "goose",
			kind = "process",
			path = root .. "/goose_process.json",
			verify = function(value)
				return value.status.code == 0 and table.concat(value.stdout):find("goose 1.36.0", 1, true)
			end,
		},
		{
			name = "kimi",
			kind = "process",
			path = root .. "/kimi_process.json",
			verify = function(value)
				return value.status.code == 0 and table.concat(value.stdout):find("kimi 1.45.0", 1, true)
			end,
		},
		{
			name = "copilot",
			kind = "jsonrpc",
			path = root .. "/copilot_acp.jsonl",
			verify = function(value)
				return value.count == 5 and value.records[4].result.sessionId == "copilot-fixture"
			end,
		},
		{
			name = "pi",
			kind = "jsonl",
			path = root .. "/pi_rpc.jsonl",
			verify = function(value)
				return value.count == 5 and value.records[1].data.sessionId == "pi-fixture"
			end,
		},
		{
			name = "vibe",
			kind = "process",
			path = root .. "/vibe_process.json",
			verify = function(value)
				return value.status.code == 0 and table.concat(value.stdout):find("vibe 2.1.0", 1, true)
			end,
		},
	},
})

assert(
	#results == 11 and results[1].name == "aider" and results[11].name == "vibe",
	"conformance must retain matrix order"
)

local duplicate = pcall(fixtures.conform, {
	cases = {
		{
			name = "fixture",
			kind = "jsonl",
			path = root .. "/stream.jsonl",
			verify = function()
				return true
			end,
		},
		{
			name = "fixture",
			kind = "jsonl",
			path = root .. "/stream.jsonl",
			verify = function()
				return true
			end,
		},
	},
})
assert(not duplicate, "conformance must reject duplicate provider names")

local failed = pcall(fixtures.conform, {
	cases = {
		{
			name = "fixture",
			kind = "jsonl",
			path = root .. "/stream.jsonl",
			verify = function()
				return false
			end,
		},
	},
})
assert(not failed, "conformance must expose failed fixture expectations")
