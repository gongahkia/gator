local M = {
	name = "extensions",
	api_version = 2,
	adapter_sdk = require("gator.extensions.adapter_sdk"),
	events = require("gator.extensions.events"),
	policy_sdk = require("gator.extensions.policy_sdk"),
	retrieval_sdk = require("gator.extensions.retrieval_sdk"),
	ui_sdk = require("gator.extensions.ui_sdk"),
}

return M
