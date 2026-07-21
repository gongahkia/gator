local adapters = require("gator").module("adapters")
local pack = require("gator").module("context").pack
local supported = { available = true, modes = { "native" } }
local contract = adapters.capabilities.new({
	provider = "opencode",
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
	id = "pack-opencode",
	task_id = "task-opencode",
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
			id = "selection-one",
			kind = "selection",
			ref = "buffer://1#L1-L2",
			provenance = { source = "editor", ref = "1" },
			trust = "manual",
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
assert(adapters.opencode_context.submit({
	pack = context,
	capabilities = contract,
	send = function(value)
		sent = value
	end,
}) == 4, "OpenCode context submission must preserve supported entries")
assert(sent.entries[3].provenance.source == "lsp", "OpenCode context submission must retain provenance")
local command
adapters.opencode_context.command({
	capabilities = contract,
	name = "review",
	advertised = { "review" },
	execute = function(value)
		command = value
	end,
})
assert(command == "review", "advertised OpenCode commands must execute")
local ineligible = pack.new({
	id = "pack-opencode-blocked",
	task_id = "task-opencode",
	entries = {
		{
			id = "blocked-file",
			kind = "file",
			ref = "SECRET.md",
			provenance = { source = "repository", ref = "HEAD" },
			trust = "repository",
			token_estimate = { status = "estimated", tokens = 1 },
			transfer = { eligible = false, reason = "policy" },
		},
	},
})
assert(not pcall(adapters.opencode_context.submit, {
	pack = ineligible,
	capabilities = contract,
	send = function() end,
}), "OpenCode context submission must reject policy-ineligible transfers")
assert(not pcall(adapters.opencode_context.command, {
	capabilities = contract,
	name = "hidden",
	advertised = {},
	execute = function() end,
}), "unadvertised OpenCode commands must fail explicitly")
