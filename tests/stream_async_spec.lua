local stream = require("gator").module("adapters").stream
local queue, events, completion = {}, {}, nil
local parser = stream.new({
	on_event = function(value)
		table.insert(events, value)
	end,
	on_complete = function(value)
		completion = value
	end,
})
assert(
	parser:feed_async('{"type":"message","text":"one"}\n{"type":"message","text":"two"}\n{"type":"complete"}\n', {
		batch_size = 1,
		schedule = function(callback)
			table.insert(queue, callback)
		end,
	}),
	"large streams must schedule a cooperative drain"
)
assert(#events == 0 and not completion, "scheduled stream parsing must not block the caller")
while #queue > 0 do
	table.remove(queue, 1)()
end
assert(
	#events == 2 and completion and parser:finish(),
	"scheduled stream drains must retain event order and completion"
)
