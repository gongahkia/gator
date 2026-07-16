local M = {
	name = "extensions",
	api_version = 1,
	adapter_sdk = require("gator.extensions.adapter_sdk"),
	events = require("gator.extensions.events"),
	manager = require("gator.extensions.manager"),
	policy_sdk = require("gator.extensions.policy_sdk"),
	retrieval_sdk = require("gator.extensions.retrieval_sdk"),
	ui_sdk = require("gator.extensions.ui_sdk"),
	workflow_sdk = require("gator.extensions.workflow_sdk"),
}

return M
