local M = {
	name = "core",
	api_version = 1,
	database = require("gator.core.database"),
	lifecycle = require("gator.core.lifecycle"),
	retention = require("gator.core.retention"),
	run = require("gator.core.run"),
	supervisor = require("gator.core.supervisor"),
	session = require("gator.core.session"),
	session_metadata = require("gator.core.session_metadata"),
	task = require("gator.core.task"),
	thread = require("gator.core.thread"),
}

return M
