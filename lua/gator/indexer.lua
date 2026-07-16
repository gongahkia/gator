local M = {
	name = "indexer",
	api_version = 1,
	lifecycle = require("gator.indexer.lifecycle"),
	protocol = require("gator.indexer.protocol"),
}

return M
