local M = {
	name = "context",
	api_version = 1,
	diagnostic = require("gator.context.diagnostic"),
	file = require("gator.context.file"),
	pack = require("gator.context.pack"),
}

return M
