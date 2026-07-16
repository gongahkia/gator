local M = {
	name = "policy",
	api_version = 1,
	file = require("gator.policy.file"),
	overlay = require("gator.policy.overlay"),
	project = require("gator.policy.project"),
	redact = require("gator.policy.redact"),
	repository = require("gator.policy.repository"),
	run = require("gator.policy.run"),
}

return M
