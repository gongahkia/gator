local M = {
	name = "core",
	api_version = 1,
	database = require("gator.core.database"),
	filesystem = require("gator.core.filesystem"),
	git = require("gator.core.git"),
	json_backend = require("gator.core.json_backend"),
	lifecycle = require("gator.core.lifecycle"),
	launch_queue = require("gator.core.launch_queue"),
	provider_event = require("gator.core.provider_event"),
	retention = require("gator.core.retention"),
	runtime = require("gator.core.runtime"),
	run = require("gator.core.run"),
	supervisor = require("gator.core.supervisor"),
	session = require("gator.core.session"),
	session_metadata = require("gator.core.session_metadata"),
	storage = require("gator.core.storage"),
	task = require("gator.core.task"),
	task_file = require("gator.core.task_file"),
	thread = require("gator.core.thread"),
}

return M
