local M = {
	name = "performance",
	api_version = 1,
	platform = require("gator.performance.platform"),
	retrieval = require("gator.performance.retrieval"),
	suite = require("gator.performance.suite"),
}

return M
