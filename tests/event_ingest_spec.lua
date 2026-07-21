local ingest = require("gator").module("core").event_ingest
local event = require("gator").module("core").provider_event
local runtime = require("gator").module("core").runtime

local scheduled, received = {}, {}
local service = ingest.new({
	runtime = runtime.new(),
	id = "provider-events",
	schedule = function(callback)
		table.insert(scheduled, callback)
	end,
	sink = {
		append = function(_, value)
			table.insert(received, value)
		end,
	},
})
local value = event.new({
	schema_version = 1,
	id = "event-ingest-one",
	run_id = "run-ingest",
	provider = { name = "codex", session_id = "native-ingest" },
	sequence = 0,
	type = "message.delta",
	at = 1,
	payload = { text = "token: private-value" },
})
assert(not pcall(service.submit, service, value), "event ingestion must require a running runtime service")
service:start()
assert(
	service:submit(value).pending == 1 and #received == 0 and #scheduled == 1,
	"normalized provider events must schedule non-blocking ingestion"
)
scheduled[1]()
assert(
	#received == 1
		and received[1].provider.session_id == "native-ingest"
		and received[1].payload.text:find("private%-value") == nil,
	"event ingestion must preserve native session identity and redacted payloads"
)
local failing_scheduled = {}
local failing = ingest.new({
	runtime = runtime.new(),
	id = "failing-events",
	schedule = function(callback)
		table.insert(failing_scheduled, callback)
	end,
	sink = {
		append = function()
			error("token: private-value")
		end,
	},
})
failing:start()
failing:submit(value)
failing_scheduled[1]()
assert(
	failing:status().pending == 1 and failing:status().last_error:find("private%-value") == nil,
	"event ingestion failures must retain pending work and redact diagnostics"
)
assert(
	failing:stop("cancelled").state == "cancelled" and failing:status().pending == 0,
	"event ingestion must cancel queued work"
)
