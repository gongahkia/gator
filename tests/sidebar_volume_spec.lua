local sidebar = require("gator.ui").sidebar
local window = sidebar.open({
	sessions = { { task_id = "task-volume", provider = "codex", id = "native-volume", streaming = false } },
	on_input = function() end,
})

for index = 1, 500 do
	sidebar.update({
		{ task_id = "task-volume", provider = "codex", id = "native-volume", streaming = index % 2 == 1 },
		{ task_id = "task-volume", provider = "claude", id = "native-volume-two", streaming = false },
	})
end
assert(sidebar.inspect().refresh_pending, "high-volume sidebar updates must coalesce pending renders")
assert(
	vim.wait(100, function()
		return not sidebar.inspect().refresh_pending
	end),
	"coalesced sidebar updates must eventually render"
)
local content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(
	sidebar.inspect().sessions == 2 and sidebar.inspect().selected == 1 and content:find("codex · idle", 1, true),
	"sidebar synchronization must retain the final event state and selected provider-native session"
)
assert(sidebar.close(), "high-volume sidebar tests must close the panel")
