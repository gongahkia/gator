local adapters = require("gator").module("adapters")
local pack = require("gator").module("context").pack
local supported = { available = true, modes = { "native" } }
local contract = adapters.capabilities.new({
	provider = "claude",
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
	id = "pack-claude",
	task_id = "task-claude",
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
			id = "diagnostic-one",
			kind = "diagnostic",
			ref = "diag://one",
			provenance = { source = "lsp", ref = "1" },
			trust = "provenance",
			token_estimate = { status = "estimated", tokens = 1 },
			transfer = { eligible = true },
		},
	},
})
local sent
assert(adapters.claude_context.submit({
	pack = context,
	capabilities = contract,
	send = function(value)
		sent = value
	end,
}) == 2, "Claude context submission must preserve eligible entries")
assert(sent.entries[2].provenance.source == "lsp", "Claude context submission must retain provenance")
local command
adapters.claude_context.command({
	capabilities = contract,
	name = "review",
	advertised = { "review" },
	execute = function(value)
		command = value
	end,
})
assert(command == "review", "advertised Claude commands must execute")
local ok = pcall(
	adapters.claude_context.command,
	{ capabilities = contract, name = "hidden", advertised = {}, execute = function() end }
)
assert(not ok, "unadvertised Claude commands must fail explicitly")
