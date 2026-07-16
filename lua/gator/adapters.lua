local M = {
	name = "adapters",
	api_version = 1,
	capabilities = require("gator.adapters.capabilities"),
	auth = require("gator.adapters.auth"),
	fixtures = require("gator.adapters.fixtures"),
	process = require("gator.adapters.process"),
	rpc = require("gator.adapters.rpc"),
	resume = require("gator.adapters.resume"),
	stream = require("gator.adapters.stream"),
	terminal = require("gator.adapters.terminal"),
}

return M
