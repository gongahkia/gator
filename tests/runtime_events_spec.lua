local core = require("gator").module("core")
local events = core.runtime_events
local provider_event = core.provider_event
local runtime = core.runtime

local function event(id, sequence, event_type)
	return provider_event.new({
		schema_version = provider_event.schema_version,
		id = id,
		run_id = "run-runtime-events",
		provider = { name = "codex", session_id = "native-runtime-events" },
		sequence = sequence,
		type = event_type,
		at = sequence,
		payload = { text = "token: private-value" },
	})
end

local callbacks, received = {}, {}
local bus = events.new({
	runtime = runtime.new(),
	schedule = function(callback)
		table.insert(callbacks, callback)
	end,
})
local subscription = bus:subscribe({
	types = { "message.delta" },
	handler = function(value)
		table.insert(received, value)
		value.payload.text = "changed"
	end,
})
assert(
	bus:publish(event("runtime-event-before-start", 0, "message.delta")).state == "unavailable",
	"event buses must report unavailable before start"
)
bus:start()
bus:append(event("runtime-event-one", 1, "message.delta"))
bus:publish(event("runtime-event-two", 2, "tool.call"))
table.remove(callbacks, 1)()
assert(
	#received == 1
		and received[1].provider.session_id == "native-runtime-events"
		and not received[1].payload.text:find("private%-value")
		and bus:status().delivered == 1,
	"runtime subscriptions must filter, redact, and isolate provider-native events"
)
assert(subscription:cancel() and not subscription:cancel(), "runtime event subscriptions must cancel idempotently")

local failed = bus:subscribe({
	handler = function()
		error("token: private-value")
	end,
})
bus:publish(event("runtime-event-failed", 3, "message.delta"))
table.remove(callbacks, 1)()
assert(
	bus:status().failures == 1 and not bus:status().last_failure:find("private%-value"),
	"runtime listener failures must remain observable and redacted"
)
failed:cancel()
assert(bus:stop("cancelled").state == "cancelled", "runtime event buses must support cancellation")
