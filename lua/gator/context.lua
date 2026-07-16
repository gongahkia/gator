local M = {
	name = "context",
	api_version = 1,
	automatic = require("gator.context.automatic"),
	diagnostic = require("gator.context.diagnostic"),
	diff = require("gator.context.diff"),
	estimate = require("gator.context.estimate"),
	file = require("gator.context.file"),
	inspect = require("gator.context.inspect"),
	lexical = require("gator.context.lexical"),
	pack = require("gator.context.pack"),
}

return M
