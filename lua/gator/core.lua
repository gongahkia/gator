local M = {
	name = "core",
	api_version = 1,
	database = require("gator.core.database"),
	filesystem = require("gator.core.filesystem"),
	lifecycle = require("gator.core.lifecycle"),
	provider_event = require("gator.core.provider_event"),
	retention = require("gator.core.retention"),
	runtime = require("gator.core.runtime"),
	run = require("gator.core.run"),
	supervisor = require("gator.core.supervisor"),
	session = require("gator.core.session"),
	session_metadata = require("gator.core.session_metadata"),
	task = require("gator.core.task"),
	thread = require("gator.core.thread"),
}

return M
