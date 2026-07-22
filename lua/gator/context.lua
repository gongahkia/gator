local M = {
	name = "context",
	api_version = 1,
	automatic = require("gator.context.automatic"),
	diagnostic = require("gator.context.diagnostic"),
	diff = require("gator.context.diff"),
	estimate = require("gator.context.estimate"),
	file = require("gator.context.file"),
	inspect = require("gator.context.inspect"),
	handoff = require("gator.context.handoff"),
	handoff_draft = require("gator.context.handoff_draft"),
	handoff_evidence = require("gator.context.handoff_evidence"),
	handoff_pack = require("gator.context.handoff_pack"),
	handoff_summary = require("gator.context.handoff_summary"),
	lexical = require("gator.context.lexical"),
	instructions = require("gator.context.instructions"),
	pack = require("gator.context.pack"),
	transfer = require("gator.context.transfer"),
	trust = require("gator.context.trust"),
}

return M
