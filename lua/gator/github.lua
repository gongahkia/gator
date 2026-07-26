local M = {
	name = "github",
	api_version = 2,
	gh = require("gator.github.gh"),
	publish = require("gator.github.publish"),
}

return M
