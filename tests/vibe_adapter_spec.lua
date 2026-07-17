local adapters = require("gator").module("adapters")
local capabilities = adapters.capabilities
local pack = require("gator").module("context").pack

local contract = capabilities.new({
	provider = "vibe",
	transport = { available = true, modes = { "acp" } },
	auth = { available = false, reason = "provider-owned" },
	session = { available = true, modes = { "create", "list", "resume", "close" } },
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
		return { sessionId = "vibe-new" }
	end
	if method == "session/list" then
		return { sessions = { { sessionId = "vibe-new" }, { sessionId = "vibe-old" } } }
	end
	return {}
end

assert(
	adapters.vibe_sessions.create({ request = request, cwd = "/tmp" }).id == "vibe-new",
	"Vibe must create ACP sessions"
)
local listed = adapters.vibe_sessions.list({ request = request, cwd = "/tmp" })
assert(#listed == 2 and listed[2].id == "vibe-old", "Vibe must preserve provider session history")
assert(
	adapters.vibe_sessions.resume({ request = request, id = "vibe-new", cwd = "/tmp" }).id == "vibe-new",
	"Vibe resume must use advertised ACP load-session support"
)
assert(adapters.vibe_sessions.close({ request = request, id = "vibe-new" }), "Vibe must close advertised ACP sessions")
assert(
	calls[1].method == "session/new"
		and calls[2].method == "session/list"
		and calls[3].method == "session/load"
		and calls[4].method == "session/close",
	"Vibe sessions must only use advertised ACP lifecycle methods"
)

local context = pack.new({
	id = "pack-vibe",
	task_id = "task-vibe",
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
assert(adapters.vibe_context.submit({
	pack = context,
	capabilities = contract,
	send = function(value)
		sent = value
	end,
}) == 1, "Vibe must transfer embedded context")
assert(sent.entries[1].ref == "README.md", "Vibe context must preserve entry references")

local launched
assert(adapters.vibe.launch({
	manager = {
		launch = function(_, opts)
			launched = opts
			return { state = "running" }
		end,
	},
	id = "vibe-run",
	cwd = vim.g.gator_test.root,
}).state == "running", "Vibe launch must use shared lifecycle management")
assert(
	launched.command[1] == "vibe-acp" and launched.cwd == vim.uv.fs_realpath(vim.g.gator_test.root),
	"Vibe launch must use the documented vibe-acp transport"
)
assert(not adapters.vibe.auth().authenticated, "Vibe authentication status must remain explicit when undocumented")
assert(not pcall(adapters.vibe.auth, { token = "secret" }), "Vibe auth must reject credential injection")
