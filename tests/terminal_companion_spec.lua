local accessibility = require("gator.ui.accessibility")
local companion = require("gator.ui.terminal_companion")

accessibility.configure({ keymaps = {}, screen_reader = false, icons = "none" })
vim.cmd("enew!")
local terminal_window = vim.api.nvim_get_current_win()
vim.api.nvim_buf_set_lines(0, 0, -1, false, { "provider secret transcript" })
local window = companion.open({
	run_id = "run-terminal-companion",
	provider = "claude",
	state = "running",
	trust = {
		surface = "terminal",
		security_owner = "provider",
		provider = "claude",
		policy = { state = "provider_owned", detail = "test", mode = "provider" },
		write = { state = "provider_owned", detail = "test", mode = "provider" },
		network = { state = "unknown", detail = "test" },
		mcp = { state = "unknown", detail = "test" },
		approval = { state = "provider_owned", detail = "test" },
	},
	workspace = { kind = "project", root = vim.fn.getcwd() },
	session = { id = "provider-session", resume_supported = true },
	started_at = os.time(),
	terminal_window = terminal_window,
	on_focus = function() end,
	on_stop = function() end,
	on_detach = function() end,
	on_runs = function() end,
	on_journal = function() end,
	on_handoff = function() end,
})
local content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(
	window ~= terminal_window
		and vim.api.nvim_get_current_win() == terminal_window
		and content:find("Transcript: unavailable", 1, true)
		and content:find("Activity: unobservable without terminal scraping", 1, true)
		and not content:find("provider secret transcript", 1, true),
	"terminal companion must be separate, restore terminal focus, and never read terminal output"
)
assert(companion.close("run-terminal-companion"), "terminal companion must close cleanly")
assert(companion.inspect("run-terminal-companion") == nil, "closed terminal companion must not remain registered")
