local M = {
	name = "core",
	api_version = 2,
	beta_readiness = require("gator.core.beta_readiness"),
	approval_event = require("gator.core.approval_event"),
	diagnostic_export = require("gator.core.diagnostic_export"),
	filesystem = require("gator.core.filesystem"),
	git = require("gator.core.git"),
	provider_status = require("gator.core.provider_status"),
}

return M
