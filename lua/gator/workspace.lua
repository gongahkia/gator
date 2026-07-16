local M = {
	name = "workspace",
	api_version = 1,
	branch = require("gator.workspace.branch"),
	links = require("gator.workspace.links"),
	monitor = require("gator.workspace.monitor"),
	policy = require("gator.workspace.policy"),
	repository = require("gator.workspace.repository"),
	scheduler = require("gator.workspace.scheduler"),
	switch = require("gator.workspace.switch"),
	worktree = require("gator.workspace.worktree"),
}

return M
