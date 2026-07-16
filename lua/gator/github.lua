local M = {
	name = "github",
	api_version = 1,
	artifacts = require("gator.github.artifacts"),
	gh = require("gator.github.gh"),
	issue = require("gator.github.issue"),
	pull_request = require("gator.github.pull_request"),
}

return M
