local details = require("gator.ui").usage_details
local usage = require("gator").module("core").usage_event

local usage_event = usage.usage({
	id = "event-usage-detail",
	run_id = "run-usage-detail",
	provider = { name = "codex", session_id = "native-usage-detail" },
	sequence = 1,
	at = 1,
	input_tokens = 12,
	output_tokens = 5,
	total_tokens = 17,
})
local compaction = usage.compaction({
	id = "event-compaction-detail",
	run_id = "run-usage-detail",
	provider = { name = "codex", session_id = "native-usage-detail" },
	sequence = 2,
	at = 2,
	before_tokens = 100,
	after_tokens = 60,
	summary = "removed token=fixture-secret from stale context",
})
local cancelled = 0
local window = details.open({
	events = { compaction, usage_event },
	on_cancel = function()
		cancelled = cancelled + 1
	end,
})
assert(vim.api.nvim_win_is_valid(window), "usage detail opening must create a window")
local content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(
	content:find("Usage · codex · session: native-usage-detail · input: 12 · output: 5 · total: 17", 1, true)
		and content:find("Context compacted · codex · session: native-usage-detail · 100 → 60", 1, true)
		and not content:find("fixture-secret", 1, true),
	"usage details must render redacted usage and compaction state with provider-native session references"
)
local selected = details.select(2)
assert(selected.id == "event-compaction-detail", "usage details must retain chronological keyboard selection")
details.update({ events = {} })
assert(details.inspect().state == "unavailable", "usage details must expose unavailable state")
assert(details.close(), "usage detail close must report success")

window = details.open({ state = "failed", reason = "provider usage stream failed", events = {} })
assert(
	table
		.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
		:find("State: failed · provider usage stream failed", 1, true),
	"usage details must expose failed state"
)
assert(details.cancel() and cancelled == 0, "failed usage panels must close without stale cancellation callbacks")
details.open({
	events = { usage_event },
	on_cancel = function()
		cancelled = cancelled + 1
	end,
})
assert(details.cancel() and cancelled == 1 and not details.inspect(), "usage detail cancellation must be explicit")
assert(not pcall(details.open, { state = "ready", events = {} }), "ready usage details must reject unavailable streams")
