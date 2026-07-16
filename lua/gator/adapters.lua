local M = {
	name = "adapters",
	api_version = 1,
	capabilities = require("gator.adapters.capabilities"),
	fixtures = require("gator.adapters.fixtures"),
	process = require("gator.adapters.process"),
}

return M
