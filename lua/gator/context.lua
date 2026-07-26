local M = {
	name = "context",
	api_version = 1,
	automatic = require("gator.context.automatic"),
	capture = require("gator.context.capture"),
	diagnostic = require("gator.context.diagnostic"),
	diff = require("gator.context.diff"),
	estimate = require("gator.context.estimate"),
	file = require("gator.context.file"),
	inspect = require("gator.context.inspect"),
	lexical = require("gator.context.lexical"),
	instructions = require("gator.context.instructions"),
	pack = require("gator.context.pack"),
	transfer = require("gator.context.transfer"),
	trust = require("gator.context.trust"),
}

return M
