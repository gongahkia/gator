local M = {
	name = "workspace",
	api_version = 1,
	branch = require("gator.workspace.branch"),
	cleanup = require("gator.workspace.cleanup"),
	collisions = require("gator.workspace.collisions"),
	links = require("gator.workspace.links"),
	lease = require("gator.workspace.lease"),
	monitor = require("gator.workspace.monitor"),
	policy = require("gator.workspace.policy"),
	recovery = require("gator.workspace.recovery"),
	repository = require("gator.workspace.repository"),
	scheduler = require("gator.workspace.scheduler"),
	snapshot = require("gator.workspace.snapshot"),
	switch = require("gator.workspace.switch"),
	worktree = require("gator.workspace.worktree"),
}

return M
