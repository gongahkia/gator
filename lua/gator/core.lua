local M = {
	name = "core",
	api_version = 2,
	beta_readiness = require("gator.core.beta_readiness"),
	approval_event = require("gator.core.approval_event"),
	diagnostic_export = require("gator.core.diagnostic_export"),
	error_event = require("gator.core.error_event"),
	event_cursor = require("gator.core.event_cursor"),
	file_event = require("gator.core.file_event"),
	filesystem = require("gator.core.filesystem"),
	git = require("gator.core.git"),
	output_excerpt = require("gator.core.output_excerpt"),
	provider_availability = require("gator.core.provider_availability"),
	provider_event = require("gator.core.provider_event"),
	provider_status = require("gator.core.provider_status"),
	retention = require("gator.core.retention"),
	run_event = require("gator.core.run_event"),
	tool_event = require("gator.core.tool_event"),
	transport_faults = require("gator.core.transport_faults"),
	usage_event = require("gator.core.usage_event"),
}

return M
