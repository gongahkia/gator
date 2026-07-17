local cline = require("gator").module("adapters").cline
local fixtures = require("gator").module("adapters").fixtures
local chunks = {}
fixtures.replay_process(vim.g.gator_test.root .. "/tests/fixtures/adapters/cline_process.json", {
	stdout = function(chunk)
		table.insert(chunks, chunk)
	end,
})
local acp
fixtures.replay_jsonrpc(vim.g.gator_test.root .. "/tests/fixtures/adapters/cline_acp.jsonl", function(message)
	if message.id == 1 and message.result then
		acp = vim.json.encode(message) .. "\n"
	end
end)
local value = cline.probe({
	cwd = vim.g.gator_test.root,
	run = function(argv)
		if argv[2] == "--version" then
			return { code = 0, stdout = chunks[1] }
		end
		if argv[2] == "--help" then
			return { code = 0, stdout = chunks[2] }
		end
		return { code = 0, stdout = acp }
	end,
})
assert(
	value.available
		and value.capabilities.acp
		and value.capabilities.stdio
		and value.capabilities.structured_output
		and value.capabilities.session_create
		and value.capabilities.session_resume
		and value.capabilities.plan
		and value.capabilities.embedded_context,
	"Cline probe must expose initialized ACP and documented CLI capabilities"
)
assert(not cline.probe({
	run = function()
		return { code = 127, stdout = "" }
	end,
}).available, "missing Cline executables must fail explicitly")
local launched
local manager = {
	launch = function(_, opts)
		launched = opts
		return { state = "running" }
	end,
}
assert(
	cline.launch({ manager = manager, id = "cline-run", cwd = vim.g.gator_test.root }).state == "running",
	"Cline launch must use shared lifecycle management"
)
assert(
	launched.command[1] == "cline"
		and launched.command[2] == "--acp"
		and launched.cwd == vim.uv.fs_realpath(vim.g.gator_test.root),
	"Cline launch must use documented ACP transport and preserve native credentials"
)
assert(
	not cline.auth().authenticated and cline.auth().reason:find("provider-independent", 1, true),
	"Cline must expose unavailable authentication status explicitly"
)
assert(not pcall(cline.launch, {
	manager = manager,
	id = "cline-run",
	cwd = vim.g.gator_test.root,
	token = "secret",
}), "Cline launch must reject credential fields")
