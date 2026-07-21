local core = require("gator").module("core")
local cursor = core.event_cursor
local event = core.provider_event
local filesystem = core.filesystem
local reconnect = core.stream_reconnect
local runtime = core.runtime
local session = core.session

local files = {}
local store = cursor.open("/fixture/reconnect.json", {
	filesystem = filesystem.new({
		readable = function(path)
			return files[path] ~= nil
		end,
		read = function(path)
			return files[path]
		end,
		mkdir = function()
			return true
		end,
		write = function(path, value)
			files[path] = value
			return true
		end,
		rename = function(source, target)
			files[target], files[source] = files[source], nil
			return true
		end,
		remove = function(path)
			files[path] = nil
			return true
		end,
	}),
})
store:advance(event.new({
	schema_version = event.schema_version,
	id = "reconnect-cursor",
	run_id = "run-reconnect",
	provider = { name = "codex", session_id = "native-reconnect" },
	sequence = 0,
	type = "message.delta",
	at = 1,
}))

local callbacks, calls = {}, {}
local owner = runtime.new({
	clock = function()
		return 1
	end,
})
local value = reconnect.new({
	runtime = owner,
	cursor = store,
	run_id = "run-reconnect",
	session = session.new({
		task_id = "task-reconnect",
		provider = "codex",
		id = "native-reconnect",
		owner = "provider",
	}),
	reconnect = function(reference, resume)
		table.insert(calls, { reference = reference, resume = resume })
		return true
	end,
	schedule = function(callback)
		table.insert(callbacks, callback)
	end,
})
assert(value:status().state == "unavailable", "reconnect managers must report unavailable before their runtime starts")
assert(value:start().state == "connecting" and #calls == 0, "stream reconnects must schedule without blocking")
table.remove(callbacks, 1)()
assert(
	value:status().state == "connected"
		and calls[1].resume.after_sequence == 0
		and calls[1].resume.next_sequence == 1
		and calls[1].reference.owner == "provider",
	"reconnects must resume after the durable cursor with provider-owned session identity"
)

assert(value:disconnect("transport lost").state == "reconnecting", "connected streams must reconnect after disconnects")
table.remove(callbacks, 1)()
assert(#calls == 2 and value:status().state == "connected", "reconnects must reuse the saved cursor")

local failing = reconnect.new({
	runtime = owner,
	id = "stream-reconnect-failing",
	cursor = store,
	run_id = "run-reconnect",
	session = session.new({ task_id = "task-reconnect", provider = "codex", id = "native-failing", owner = "provider" }),
	max_attempts = 1,
	reconnect = function()
		error("token: private-value")
	end,
	schedule = function(callback)
		table.insert(callbacks, callback)
	end,
})
failing:start()
table.remove(callbacks, 1)()
assert(
	failing:status().state == "failed" and not failing:status().reason:find("private%-value"),
	"reconnect failures must be explicit and redacted"
)

assert(value:stop("user cancelled").state == "cancelled", "stream reconnects must support cancellation")
