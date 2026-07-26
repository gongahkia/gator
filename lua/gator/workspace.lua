local M = {
	name = "workspace",
	api_version = 1,
	cleanup = require("gator.workspace.cleanup"),
	collisions = require("gator.workspace.collisions"),
	lease = require("gator.workspace.lease"),
	monitor = require("gator.workspace.monitor"),
	policy = require("gator.workspace.policy"),
	repository = require("gator.workspace.repository"),
	scheduler = require("gator.workspace.scheduler"),
	snapshot = require("gator.workspace.snapshot"),
	switch = require("gator.workspace.switch"),
	worktree = require("gator.workspace.worktree"),
}

return M
