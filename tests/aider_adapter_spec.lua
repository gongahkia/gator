local adapters = require("gator").module("adapters")
local fixtures = adapters.fixtures
local policy = require("gator").module("policy").overlay
local pack = require("gator").module("context").pack
local chunks = {}
fixtures.replay_terminal(vim.g.gator_test.root .. "/tests/fixtures/adapters/aider_terminal.json", function(chunk)
	table.insert(chunks, chunk)
end)
assert(
	adapters.aider_stream.parse(table.concat(chunks)).text == "Aider: gator-fixture\n",
	"Aider terminal fixture must preserve streamed visible text"
)

local supported = { available = true, modes = { "native" } }
local contract = adapters.capabilities.new({
	provider = "aider",
	transport = supported,
	auth = { available = false, reason = "Aider status is provider-specific" },
	session = { available = false, reason = "Aider has no provider session API" },
	permission = { available = true, modes = { "mode" } },
	model = supported,
	command = supported,
	tool = supported,
	context = { available = true, modes = { "read" } },
	usage = supported,
})
local read_only = policy.new({
	scope = "run",
	target = "aider-run",
	rules = { write_allowed = false },
	provenance = { source = "policy", ref = "test" },
})
assert(
	adapters.aider_policy.map(read_only, contract).chat_mode == "ask",
	"Aider read-only policy must use native ask mode"
)
local write = policy.new({
	scope = "run",
	target = "aider-run",
	rules = { write_allowed = true },
	provenance = { source = "policy", ref = "test" },
})
assert(
	adapters.aider_policy.map(write, contract).chat_mode == "code",
	"Aider writable policy must use native code mode"
)

local context = pack.new({
	id = "pack-aider",
	task_id = "task-aider",
	entries = {
		{
			id = "file-one",
			kind = "file",
			ref = "README.md",
			provenance = { source = "repository", ref = "HEAD" },
			trust = "repository",
			token_estimate = { status = "estimated", tokens = 1 },
			transfer = { eligible = true },
		},
	},
})
local sent
assert(adapters.aider_context.submit({
	pack = context,
	capabilities = contract,
	send = function(value)
		sent = value
	end,
}) == 1, "Aider context submission must map file context")
assert(sent.read[1] == "README.md", "Aider context submission must preserve native read-file paths")
assert(
	not adapters.aider_sessions.create().available and not adapters.aider_sessions.resume().available,
	"Aider local history must not be fabricated as a provider session API"
)
