local adapters = require("gator").module("adapters")
local pack = require("gator").module("context").pack
local supported = { available = true, modes = { "native" } }
local contract = adapters.capabilities.new({
	provider = "amp",
	transport = supported,
	auth = supported,
	session = supported,
	permission = { available = false, reason = "Amp CLI permission flags are unavailable" },
	model = supported,
	command = supported,
	tool = supported,
	context = { available = true, modes = { "input" } },
	usage = supported,
})
local context = pack.new({
	id = "pack-amp",
	task_id = "task-amp",
	entries = {
		{
			id = "selection-one",
			kind = "selection",
			ref = "lua/gator/init.lua:1",
			provenance = { source = "editor", ref = "buffer-one" },
			trust = "manual",
			token_estimate = { status = "estimated", tokens = 1 },
			transfer = { eligible = true },
			content = "local M = {}",
		},
	},
})
local sent
assert(adapters.amp_context.submit({
	pack = context,
	capabilities = contract,
	send = function(value)
		sent = value
	end,
}) == 1, "Amp context submission must emit one stream-input message")
assert(
	sent.type == "user" and sent.message.role == "user" and sent.message.content[1].type == "text",
	"Amp context submission must preserve the documented stream-input shape"
)
