local stream = require("gator").module("adapters").stream
local events, errors, completion = {}, {}, nil
local parser = stream.new({
	on_event = function(value)
		table.insert(events, value)
	end,
	on_complete = function(value)
		completion = value
	end,
	on_error = function(reason, line)
		table.insert(errors, { reason = reason, line = line })
	end,
})
parser:feed('{"type":"message","text":"first"')
assert(#events == 0, "partial JSONL records must wait for their newline")
parser:feed('}\nnot json\n{"type":"complete","reason":"stop"}\n')
assert(events[1].type == "message" and events[1].data.text == "first", "JSONL events must normalize type and data")
assert(#errors == 1 and errors[1].reason == "invalid JSON", "malformed records must surface explicitly")
assert(completion.data.reason == "stop" and parser:finish(), "completion events must close complete streams")
assert(
	parser:feed('{"type":"message"}\n') == 1 and #errors == 2,
	"events after completion must surface through the error callback"
)
