local M = {
	name = "workspace",
	api_version = 1,
	policy = require("gator.workspace.policy"),
	repository = require("gator.workspace.repository"),
	worktree = require("gator.workspace.worktree"),
}

return M
