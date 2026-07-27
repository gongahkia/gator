local accessibility = require("gator.ui.accessibility")
local events = require("gator.ui.run_events")

accessibility.configure({ keymaps = {}, screen_reader = false, icons = "none" })
local window = events.open({
	run = { id = "run-events-ui" },
	events = {
		{ sequence = 0, type = "context.prepared", at = 1, payload = { bytes = 12, tokens = 3 } },
		{ sequence = 1, type = "approval.decided", at = 2, payload = { decision = "denied" } },
	},
})
local content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(
	content:find("Gator event journal · run-events-ui", 1, true)
		and content:find("context.prepared", 1, true)
		and content:find("approval.decided", 1, true),
	"journal UI must list immutable Gator-owned lifecycle events"
)
assert(events.close(), "journal UI must remain ephemeral")
