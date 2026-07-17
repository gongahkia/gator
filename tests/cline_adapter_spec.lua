local adapters = require("gator").module("adapters")
local fixtures = adapters.fixtures
local policy = require("gator").module("policy").overlay
local pack = require("gator").module("context").pack
local stream = {}
fixtures.replay_jsonl(vim.g.gator_test.root .. "/tests/fixtures/adapters/cline_stream.jsonl", function(record)
	table.insert(stream, vim.json.encode(record))
end)
local events = adapters.cline_stream.parse(table.concat(stream, "\n"))
assert(
	#events == 2 and events[1].text == "gator-" and events[2].partial == false,
	"Cline stream parser must preserve documented JSON messages"
)

local supported = { available = true, modes = { "native" } }
local contract = adapters.capabilities.new({
	provider = "cline",
	transport = supported,
	auth = { available = false, reason = "Cline auth status is provider-specific" },
	session = { available = true, modes = { "create", "resume" } },
	permission = { available = true, modes = { "mode" } },
	model = supported,
	command = supported,
	tool = supported,
	context = { available = true, modes = { "embedded" } },
	usage = supported,
})
local read_only = policy.new({
	scope = "run",
	target = "cline-run",
	rules = { write_allowed = false },
	provenance = { source = "policy", ref = "test" },
})
assert(
	vim.deep_equal(adapters.cline_policy.map(read_only, contract).args, { "--plan", "--auto-approve", "false" }),
	"Cline read-only policy must use plan mode and disable auto-approval"
)
local write = policy.new({
	scope = "run",
	target = "cline-run",
	rules = { write_allowed = true },
	provenance = { source = "policy", ref = "test" },
})
assert(
	vim.deep_equal(adapters.cline_policy.map(write, contract).args, { "--auto-approve", "false" }),
	"Cline writable policy must preserve native per-tool approval"
)

local context = pack.new({
	id = "pack-cline",
	task_id = "task-cline",
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
assert(adapters.cline_context.submit({
	pack = context,
	capabilities = contract,
	send = function(value)
		sent = value
	end,
}) == 1, "Cline context submission must preserve transferable context")
assert(sent.entries[1].ref == "README.md", "Cline context must preserve ACP entry references")

local request = function(method, params)
	assert(method == "session/new" and params.mcpServers[1] == nil, "Cline creation must use ACP")
	return { sessionId = "cline-fixture" }
end
assert(
	adapters.cline_sessions.create({ request = request, cwd = "/tmp" }).id == "cline-fixture",
	"Cline session creation must preserve ACP session ids"
)
assert(adapters.cline_sessions.resume({
	run = function(argv)
		assert(
			argv[2] == "--id" and argv[3] == "cline-fixture" and argv[4] == "--json" and argv[5] == "continue",
			"Cline resume must use documented --id and --json flags"
		)
		return { code = 0, stdout = table.concat(stream, "\n") }
	end,
	id = "cline-fixture",
	prompt = "continue",
}).id == "cline-fixture", "Cline resume must preserve provider session ids")
assert(
	not adapters.cline_sessions.list().available and not adapters.cline_sessions.close().available,
	"Cline undocumented session lifecycle operations must remain unavailable"
)
