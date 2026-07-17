local adapters = require("gator").module("adapters")
local capabilities = adapters.capabilities
local pack = require("gator").module("context").pack

local contract = capabilities.new({
	provider = "goose",
	transport = { available = true, modes = { "acp" } },
	auth = { available = false, reason = "provider-owned" },
	session = { available = true, modes = { "create", "list", "resume", "close" } },
	permission = { available = false, reason = "interactive-only" },
	model = { available = false, reason = "provider-owned" },
	command = { available = false, reason = "provider-owned" },
	tool = { available = false, reason = "provider-owned" },
	context = { available = true, modes = { "embedded" } },
	usage = { available = false, reason = "provider-owned" },
})

local request_calls = {}
local function request(method, params)
	table.insert(request_calls, { method = method, params = params })
	if method == "session/new" then
		return { sessionId = "goose-new" }
	end
	if method == "session/list" then
		return { sessions = { { sessionId = "goose-new" }, { sessionId = "goose-old" } } }
	end
	return {}
end

assert(
	adapters.goose_sessions.create({ request = request, cwd = "/tmp" }).id == "goose-new",
	"Goose must create ACP sessions"
)
local listed = adapters.goose_sessions.list({ request = request, cwd = "/tmp" })
assert(#listed == 2 and listed[2].id == "goose-old", "Goose must preserve provider session history")
assert(
	adapters.goose_sessions.resume({ request = request, id = "goose-new", cwd = "/tmp" }).id == "goose-new",
	"Goose resume must use advertised ACP load-session support"
)
assert(
	adapters.goose_sessions.close({ request = request, id = "goose-new" }),
	"Goose must close advertised ACP sessions"
)
assert(
	request_calls[1].method == "session/new"
		and request_calls[2].method == "session/list"
		and request_calls[3].method == "session/load"
		and request_calls[4].method == "session/close",
	"Goose sessions must only use advertised ACP lifecycle methods"
)

local context = pack.new({
	id = "pack-goose",
	task_id = "task-goose",
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
assert(adapters.goose_context.submit({
	pack = context,
	capabilities = contract,
	send = function(value)
		sent = value
	end,
}) == 1, "Goose must transfer embedded context")
assert(sent.entries[1].ref == "README.md", "Goose context must preserve entry references")

local launched
assert(adapters.goose.launch({
	manager = {
		launch = function(_, opts)
			launched = opts
			return { state = "running" }
		end,
	},
	id = "goose-run",
	cwd = vim.g.gator_test.root,
}).state == "running", "Goose launch must use shared lifecycle management")
assert(
	launched.command[1] == "goose"
		and launched.command[2] == "acp"
		and launched.cwd == vim.uv.fs_realpath(vim.g.gator_test.root),
	"Goose launch must use documented ACP transport"
)
assert(not adapters.goose.auth().authenticated, "Goose authentication status must remain explicit when undocumented")
assert(not pcall(adapters.goose.auth, { token = "secret" }), "Goose auth must reject credential injection")
