local details = require("gator.ui").event_details
local event = require("gator").module("core").provider_event

local function message(id, sequence, event_type, payload)
	return event.new({
		schema_version = event.schema_version,
		id = id,
		run_id = "run-event-details",
		provider = { name = "opencode", session_id = "native-event-details" },
		sequence = sequence,
		type = event_type,
		at = sequence,
		payload = payload,
	})
end

local cancelled = 0
local window = details.open({
	events = {
		message("event-thought", 2, "message.thought", { text = "checking token=fixture-secret" }),
		message("event-message", 1, "message.delta", { text = "implemented the panel" }),
		message("event-tool", 3, "tool.call", { name = "ignored" }),
	},
	on_cancel = function()
		cancelled = cancelled + 1
	end,
})
assert(vim.api.nvim_win_is_valid(window), "event detail opening must create a window")
local content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(
	content:find("Message delta · opencode · session: native-event-details", 1, true)
		and content:find("Reasoning summary · opencode · session: native-event-details", 1, true)
		and not content:find("fixture-secret", 1, true),
	"event details must render redacted message and reasoning summaries with provider-owned session references"
)
local selected = details.select(2)
assert(
	selected.id == "event-thought" and details.inspect().selected == 2,
	"event details must support keyboard-addressable selection"
)
details.update({ events = {} })
assert(
	details.inspect().state == "unavailable"
		and table
			.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
			:find("no message or reasoning events", 1, true),
	"event details must expose unavailable state"
)
assert(details.close(), "event detail close must report success")

window = details.open({ state = "failed", reason = "provider stream failed", events = {} })
assert(
	table
		.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
		:find("State: failed · provider stream failed", 1, true),
	"event details must expose provider failures"
)
assert(
	details.cancel() and cancelled == 0,
	"event detail cancellation must close failed panels without stale callbacks"
)

details.open({
	events = { message("event-cancel", 4, "message.completed", { text = "done" }) },
	on_cancel = function()
		cancelled = cancelled + 1
	end,
})
assert(
	details.cancel() and cancelled == 1 and not details.inspect(),
	"event detail cancellation must be explicit and idempotently close the panel"
)
assert(
	not pcall(details.open, { state = "ready", events = {} }),
	"ready event details must reject unavailable message streams"
)
assert(
	not pcall(details.open, { events = { { type = "message.delta" } } }),
	"event details must require normalized provider events"
)
