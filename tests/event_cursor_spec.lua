local cursor = require("gator").module("core").event_cursor
local event = require("gator").module("core").provider_event
local filesystem = require("gator").module("core").filesystem

local files = {}
local store = cursor.open("/fixture/cursors.json", {
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
local first = event.new({
	schema_version = 1,
	id = "event-cursor-one",
	run_id = "run-cursor",
	provider = { name = "codex", session_id = "native-cursor" },
	sequence = 0,
	type = "message.delta",
	at = 1,
})
assert(store:advance(first) == 0 and store:get("run-cursor") == 0, "event cursors must persist accepted sequence zero")
local reopened = cursor.open("/fixture/cursors.json", { filesystem = store.filesystem })
assert(reopened:get("run-cursor") == 0, "event cursors must survive storage reopens")
assert(not pcall(reopened.advance, reopened, first), "event cursors must reject duplicate provider events")
local gap = event.new({
	schema_version = 1,
	id = "event-cursor-gap",
	run_id = "run-cursor",
	provider = { name = "codex" },
	sequence = 2,
	type = "message.delta",
	at = 2,
})
assert(not pcall(reopened.advance, reopened, gap), "event cursors must reject sequence gaps")
