local M = {
	name = "github",
	api_version = 1,
	gh = require("gator.github.gh"),
	issue = require("gator.github.issue"),
	pull_request = require("gator.github.pull_request"),
}

return M
