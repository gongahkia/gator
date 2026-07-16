local M = {
	name = "indexer",
	api_version = 1,
	health = require("gator.indexer.health"),
	lifecycle = require("gator.indexer.lifecycle"),
	protocol = require("gator.indexer.protocol"),
}

return M
