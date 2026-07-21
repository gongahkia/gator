local usage = require("gator").module("core").usage_event

local update = usage.usage({
	id = "event-usage",
	run_id = "run-usage",
	provider = { name = "codex" },
	sequence = 0,
	at = 1,
	input_tokens = 2,
	output_tokens = 3,
	total_tokens = 5,
})
local compacted = usage.compaction({
	id = "event-compact",
	run_id = "run-usage",
	provider = { name = "codex" },
	sequence = 1,
	at = 2,
	before_tokens = 10,
	after_tokens = 4,
	summary = "token: private-value",
})
assert(
	update.type == "usage.update"
		and compacted.type == "context.compacted"
		and compacted.payload.summary:find("private%-value") == nil,
	"usage and compaction events must normalize redacted provider data"
)
assert(not pcall(usage.compaction, {
	id = "event-compact-invalid",
	run_id = "run-usage",
	provider = { name = "codex" },
	sequence = 2,
	at = 3,
	before_tokens = 1,
	after_tokens = 2,
}), "compaction events must reject token growth")
