local M = {
	name = "telemetry",
	api_version = 1,
	aggregate = require("gator.telemetry.aggregate"),
	consent = require("gator.telemetry.consent"),
	event = require("gator.telemetry.event"),
	log = require("gator.telemetry.log"),
}

return M
