local M = {
	name = "core",
	api_version = 1,
	lifecycle = require("gator.core.lifecycle"),
	session = require("gator.core.session"),
	session_metadata = require("gator.core.session_metadata"),
	task = require("gator.core.task"),
	thread = require("gator.core.thread"),
}

return M
