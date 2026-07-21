local ingest = require("gator").module("core").event_ingest
local event = require("gator").module("core").provider_event
local runtime = require("gator").module("core").runtime

local function cursor()
	local values = {}
	return {
		assert_next = function(_, value)
			assert(value.sequence == (values[value.run_id] or -1) + 1, "event sequence must be contiguous")
		end,
		advance = function(_, value)
			values[value.run_id] = value.sequence
		end,
	}
end

local scheduled, received = {}, {}
local service = ingest.new({
	runtime = runtime.new(),
	id = "provider-events",
	cursor = cursor(),
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
	cursor = cursor(),
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

local bounded = ingest.new({
	runtime = runtime.new(),
	id = "bounded-events",
	cursor = cursor(),
	limit = 1,
	schedule = function() end,
	sink = {
		append = function() end,
	},
})
bounded:start()
bounded:submit(value)
assert(not pcall(bounded.submit, bounded, value), "event ingestion must reject overflow by default")
local replacement = event.new({
	schema_version = 1,
	id = "event-ingest-two",
	run_id = "run-ingest",
	provider = { name = "codex" },
	sequence = 0,
	type = "message.delta",
	at = 2,
})
local newest_scheduled, newest_received = {}, {}
local newest = ingest.new({
	runtime = runtime.new(),
	id = "drop-oldest-events",
	cursor = cursor(),
	limit = 1,
	overflow = "drop_oldest",
	schedule = function(callback)
		table.insert(newest_scheduled, callback)
	end,
	sink = {
		append = function(_, item)
			table.insert(newest_received, item)
		end,
	},
})
newest:start()
newest:submit(value)
newest:submit(replacement)
newest_scheduled[1]()
assert(
	newest:status().dropped == 1 and newest_received[1].id == "event-ingest-two",
	"event ingestion must apply configured overflow policy without blocking providers"
)
