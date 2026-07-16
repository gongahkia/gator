local M = {
	name = "adapters",
	api_version = 1,
	capabilities = require("gator.adapters.capabilities"),
	codex = require("gator.adapters.codex"),
	codex_sessions = require("gator.adapters.codex_sessions"),
	codex_policy = require("gator.adapters.codex_policy"),
	auth = require("gator.adapters.auth"),
	fixtures = require("gator.adapters.fixtures"),
	model_usage = require("gator.adapters.model_usage"),
	process = require("gator.adapters.process"),
	rpc = require("gator.adapters.rpc"),
	resume = require("gator.adapters.resume"),
	stream = require("gator.adapters.stream"),
	terminal = require("gator.adapters.terminal"),
}

return M
