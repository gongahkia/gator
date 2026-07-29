local M = {
	name = "context",
	api_version = 1,
	capture = require("gator.context.capture"),
	estimate = require("gator.context.estimate"),
	references = require("gator.context.references"),
}

return M
