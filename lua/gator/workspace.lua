local M = {
	name = "workspace",
	api_version = 1,
	policy = require("gator.workspace.policy"),
	repository = require("gator.workspace.repository"),
}

return M
