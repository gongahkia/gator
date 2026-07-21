local stream = require("gator").module("adapters").stream
local core = require("gator").module("core")

local callbacks, events, completed = {}, {}, false
local parser = stream.new({
	on_event = function(value)
		table.insert(events, value.data.text)
	end,
	on_complete = function()
		completed = true
	end,
})
parser:feed_async('{"type":"message","text":"first"}\n{"type":"message","text":"second"}\n{"type":"complete"}\n', {
	batch_size = 1,
	schedule = function(callback)
		table.insert(callbacks, callback)
	end,
})
while #callbacks > 0 do
	table.remove(callbacks, 1)()
end
assert(
	vim.deep_equal(events, { "first", "second" }) and completed and parser:finish(),
	"scheduled streams must preserve event order through completion"
)

local excerpt = core.output_excerpt.new({ limit = 16 })
local truncated = excerpt:append("prefix-token: private-value")
assert(
	truncated.truncated and truncated.bytes <= 16 and not truncated.text:find("private%-value"),
	"live stream excerpts must truncate after redaction"
)

local function cursor()
	local sequences = {}
	return {
		classify = function(_, value)
			local sequence = sequences[value.run_id] or -1
			if value.sequence <= sequence then
				return { status = "duplicate", sequence = sequence }
			end
			if value.sequence == sequence + 1 then
				return { status = "next", sequence = sequence }
			end
			return { status = "gap", sequence = sequence, expected = sequence + 1 }
		end,
		assert_next = function(_, value)
			assert(value.sequence == (sequences[value.run_id] or -1) + 1)
		end,
		advance = function(_, value)
			sequences[value.run_id] = value.sequence
		end,
	}
end

local function event(id, sequence)
	return core.provider_event.new({
		schema_version = core.provider_event.schema_version,
		id = id,
		run_id = "run-stream-contract",
		provider = { name = "codex", session_id = "native-stream-contract" },
		sequence = sequence,
		type = "message.delta",
		at = sequence,
		payload = { text = id },
	})
end

local drains, received = {}, {}
local ingest = core.event_ingest.new({
	runtime = core.runtime.new(),
	id = "stream-contract",
	cursor = cursor(),
	limit = 2,
	schedule = function(callback)
		table.insert(drains, callback)
	end,
	sink = {
		append = function(_, value)
			table.insert(received, value.id)
		end,
	},
})
ingest:start()
ingest:submit(event("stream-contract-one", 0))
ingest:submit(event("stream-contract-two", 1))
assert(
	not pcall(ingest.submit, ingest, event("stream-contract-overflow", 2)),
	"bounded ingestion must apply backpressure before accepting an overflow event"
)
table.remove(drains, 1)()
assert(
	vim.deep_equal(received, { "stream-contract-one", "stream-contract-two" }) and ingest:status().pending == 0,
	"backpressured ingestion must retain accepted event ordering"
)
