local core = require("gator").module("core")
local fanout = core.event_fanout
local provider_event = core.provider_event
local runtime = core.runtime

local function event(id, sequence)
	return provider_event.new({
		schema_version = provider_event.schema_version,
		id = id,
		run_id = "run-fanout",
		provider = { name = "codex", session_id = "native-fanout" },
		sequence = sequence,
		type = "tool.call",
		at = sequence,
		payload = { name = "read_file" },
	})
end

local callbacks, sidebar, panel = {}, {}, {}
local focused = true
local owner = runtime.new({
	clock = function()
		return 1
	end,
})
local value = fanout.new({
	runtime = owner,
	id = "provider-fanout",
	sidebar = function(received)
		table.insert(sidebar, received)
	end,
	panel = function(received)
		table.insert(panel, received)
	end,
	focused = function()
		return focused
	end,
	schedule = function(callback)
		table.insert(callbacks, callback)
	end,
})
assert(
	value:submit(event("fanout-before-start", 0)).state == "unavailable",
	"fanout must report unavailable before start"
)

value:start()
assert(value:submit(event("fanout-first", 1)).pending == 1 and #sidebar == 0, "fanout must schedule without blocking")
table.remove(callbacks, 1)()
assert(
	#sidebar == 1 and #panel == 1 and sidebar[1].provider.session_id == "native-fanout",
	"fanout must preserve redacted provider event ownership for sidebar and focused panels"
)

focused = false
value:submit(event("fanout-background", 2))
table.remove(callbacks, 1)()
assert(#sidebar == 2 and #panel == 1, "fanout must not update unfocused panels")

local failing = fanout.new({
	runtime = owner,
	id = "provider-fanout-failing",
	sidebar = function() end,
	panel = function() end,
	focused = function()
		return true
	end,
	schedule = function()
		error("token: private-value")
	end,
})
failing:start()
local failed = failing:submit(event("fanout-failed", 3))
assert(
	failed.state == "failed" and not failed.reason:find("private%-value"),
	"fanout scheduling failures must be explicit and redacted"
)

assert(value:stop("user cancelled").state == "cancelled", "fanout must support runtime cancellation")
assert(value:submit(event("fanout-cancelled", 4)).state == "cancelled", "cancelled fanout must reject later events")
