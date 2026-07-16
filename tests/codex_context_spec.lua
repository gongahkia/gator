local adapters = require("gator").module("adapters")
local pack = require("gator").module("context").pack
local supported = { available = true, modes = { "native" } }
local contract = adapters.capabilities.new({
	provider = "codex",
	transport = supported,
	auth = supported,
	session = supported,
	permission = supported,
	model = supported,
	command = { available = true, modes = { "execute" } },
	tool = supported,
	context = { available = true, modes = { "attach" } },
	usage = supported,
})
local context = pack.new({
	id = "pack-codex",
	task_id = "task-codex",
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
		{
			id = "diff-one",
			kind = "diff",
			ref = "diff://one",
			provenance = { source = "repository", ref = "HEAD" },
			trust = "repository",
			token_estimate = { status = "estimated", tokens = 1 },
			transfer = { eligible = true },
		},
	},
})
local sent
assert(adapters.codex_context.submit({
	pack = context,
	capabilities = contract,
	send = function(value)
		sent = value
	end,
}) == 2, "Codex context submission must preserve eligible entries")
assert(sent.entries[2].provenance.ref == "HEAD", "Codex context submission must retain provenance")
local command
adapters.codex_context.command({
	capabilities = contract,
	name = "review",
	advertised = { "review" },
	execute = function(value)
		command = value
	end,
})
assert(command == "review", "advertised Codex commands must execute")
local ok = pcall(
	adapters.codex_context.command,
	{ capabilities = contract, name = "hidden", advertised = {}, execute = function() end }
)
assert(not ok, "unadvertised Codex commands must fail explicitly")
