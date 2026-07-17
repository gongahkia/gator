local adapters = require("gator").module("adapters")
local capabilities = adapters.capabilities
local pack = require("gator").module("context").pack

local contract = capabilities.new({
	provider = "kimi",
	transport = { available = true, modes = { "acp" } },
	auth = { available = false, reason = "provider-owned" },
	session = { available = true, modes = { "create", "list", "resume" } },
	permission = { available = false, reason = "provider-owned" },
	model = { available = false, reason = "provider-owned" },
	command = { available = false, reason = "provider-owned" },
	tool = { available = false, reason = "provider-owned" },
	context = { available = true, modes = { "embedded" } },
	usage = { available = false, reason = "provider-owned" },
})

local calls = {}
local function request(method, params)
	table.insert(calls, { method = method, params = params })
	if method == "session/new" then
		return { sessionId = "kimi-new" }
	end
	if method == "session/list" then
		return { sessions = { { sessionId = "kimi-new" }, { sessionId = "kimi-old" } } }
	end
	return {}
end

assert(
	adapters.kimi_sessions.create({ request = request, cwd = "/tmp" }).id == "kimi-new",
	"Kimi must create ACP sessions"
)
local listed = adapters.kimi_sessions.list({ request = request, cwd = "/tmp" })
assert(#listed == 2 and listed[2].id == "kimi-old", "Kimi must preserve provider session history")
assert(
	adapters.kimi_sessions.resume({ request = request, id = "kimi-new", cwd = "/tmp" }).id == "kimi-new",
	"Kimi resume must use advertised ACP load-session support"
)
assert(
	calls[1].method == "session/new" and calls[2].method == "session/list" and calls[3].method == "session/load",
	"Kimi sessions must only use advertised ACP lifecycle methods"
)
assert(not adapters.kimi_sessions.close().available, "Kimi undocumented session deletion must remain unavailable")

local context = pack.new({
	id = "pack-kimi",
	task_id = "task-kimi",
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
assert(adapters.kimi_context.submit({
	pack = context,
	capabilities = contract,
	send = function(value)
		sent = value
	end,
}) == 1, "Kimi must transfer embedded context")
assert(sent.entries[1].ref == "README.md", "Kimi context must preserve entry references")

local launched
assert(adapters.kimi.launch({
	manager = {
		launch = function(_, opts)
			launched = opts
			return { state = "running" }
		end,
	},
	id = "kimi-run",
	cwd = vim.g.gator_test.root,
}).state == "running", "Kimi launch must use shared lifecycle management")
assert(
	launched.command[1] == "kimi"
		and launched.command[2] == "acp"
		and launched.cwd == vim.uv.fs_realpath(vim.g.gator_test.root),
	"Kimi launch must use documented ACP transport"
)
assert(not adapters.kimi.auth().authenticated, "Kimi authentication status must remain explicit when undocumented")
assert(not pcall(adapters.kimi.auth, { token = "secret" }), "Kimi auth must reject credential injection")
