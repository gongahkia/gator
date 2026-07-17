local M = {
	name = "performance",
	api_version = 1,
	budgets = require("gator.performance.budgets"),
	platform = require("gator.performance.platform"),
	retrieval = require("gator.performance.retrieval"),
	suite = require("gator.performance.suite"),
}

return M
