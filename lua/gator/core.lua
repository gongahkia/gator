local M = {
	name = "core",
	api_version = 1,
	lifecycle = require("gator.core.lifecycle"),
	task = require("gator.core.task"),
}

return M
