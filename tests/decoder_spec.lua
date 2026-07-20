local decoder = require("gator").module("adapters").decoder

local value = decoder.new({
	provider = "fixture",
	event_id = function(context)
		return "fixture-event-" .. context.sequence
	end,
	now = function()
		return 7
	end,
	decode = function(raw, context)
		assert(
			context.run_id == "run-decoder" and raw.text == "fixture",
			"decoder must receive isolated raw input and run context"
		)
		return {
			{ type = "message.delta", payload = { text = "token: private-value" } },
			{ type = "run.completed", payload = { status = "complete" }, at = 8 },
		}
	end,
})

local events = value:decode({ text = "fixture" }, { run_id = "run-decoder", session_id = "native-session" })
assert(
	#events == 2
		and events[1].sequence == 0
		and events[2].sequence == 1
		and events[1].provider.session_id == "native-session"
		and events[1].payload.text:find("private%-value") == nil,
	"decoders must normalize ordered, redacted provider events"
)
assert(value:status().state == "ready", "active decoders must report ready state")
assert(value:cancel("user cancelled"), "decoders must support explicit cancellation")
assert(value:status().state == "cancelled" and not value:cancel(), "decoder cancellation must be idempotent")
assert(not pcall(value.decode, value, {}, { run_id = "run-decoder" }), "cancelled decoders must reject later events")

local unavailable = decoder.unavailable({ provider = "fixture", reason = "transport is unavailable" })
assert(
	not unavailable:status().available and not pcall(unavailable.decode, unavailable, {}, { run_id = "run-decoder" }),
	"unavailable decoders must remain explicit"
)
local invalid = decoder.new({
	provider = "fixture",
	decode = function()
		return { type = "unknown.delta" }
	end,
})
assert(
	not pcall(invalid.decode, invalid, {}, { run_id = "run-decoder" }),
	"decoder output must match the provider event contract"
)
