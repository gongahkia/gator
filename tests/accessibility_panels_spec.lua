local gator = require("gator").setup({
	ui = {
		keymaps = {
			next = "]",
			previous = "[",
			confirm = "c",
			cancel = "x",
			toggle = "t",
			accept = "a",
			reject = "r",
			prompt = "p",
			help = "h",
		},
		screen_reader = true,
	},
})
local ui = require("gator.ui")
local pack = require("gator.context.pack")
local run = require("gator.core.run")

local function mapping(window, lhs, action)
	local value = vim.api.nvim_buf_call(vim.api.nvim_win_get_buf(window), function()
		return vim.fn.maparg(lhs, "n", false, true)
	end)
	assert(
		value.buffer == 1 and value.desc == "Gator " .. action,
		"panel must expose configured buffer-local " .. action
	)
end

local function panel(window, maps)
	assert(vim.bo[vim.api.nvim_win_get_buf(window)].filetype == "gator-text", "screen-reader panels must be plain text")
	assert(not vim.bo[vim.api.nvim_win_get_buf(window)].modifiable, "screen-reader panels must be read-only")
	for lhs, action in pairs(maps) do
		mapping(window, lhs, action)
	end
end

local function press(lhs)
	vim.api.nvim_feedkeys(vim.api.nvim_replace_termcodes(lhs, true, false, true), "xt", false)
end

vim.cmd("enew")
local user_window = vim.api.nvim_get_current_win()
local user_buffer = vim.api.nvim_get_current_buf()
local window = ui.open(gator._state)
panel(window, { ["]"] = "next", ["c"] = "confirm", ["x"] = "cancel", ["h"] = "help" })
press("x")
assert(
	vim.api.nvim_get_current_win() == user_window and vim.api.nvim_win_get_buf(user_window) == user_buffer,
	"keyboard close must restore workspace focus"
)

local dashboard = ui.dashboard
local opened
window = dashboard.open({
	tasks = {
		{
			id = "task-accessibility",
			objective = "keyboard flow",
			lifecycle = "running",
			provider = "codex",
			workspace = "project",
			review_state = "pending",
		},
	},
	on_open = function(task)
		opened = task
	end,
})
panel(window, { ["]"] = "next", ["c"] = "confirm", ["x"] = "cancel", ["h"] = "help" })
press("c")
assert(opened and opened.id == "task-accessibility", "keyboard confirmation must open the selected dashboard task")
press("x")

local sidebar = ui.sidebar
window = sidebar.open({
	sessions = {
		{ task_id = "task-accessibility", provider = "codex", id = "native-accessibility", streaming = false },
	},
	on_input = function() end,
})
panel(window, { ["]"] = "next", ["p"] = "prompt", ["x"] = "cancel", ["h"] = "help" })
assert(sidebar.close(), "sidebar must close after accessibility inspection")

local timeline = ui.timeline
window = timeline.open({
	calls = {
		{
			id = "call-accessibility",
			provider = "codex",
			session_id = "native-accessibility",
			name = "read_file",
			arguments = "{}",
			approval = "granted",
			status = "succeeded",
		},
	},
})
panel(window, { ["]"] = "next", ["t"] = "toggle", ["x"] = "cancel", ["h"] = "help" })
assert(timeline.close(), "timeline must close after accessibility inspection")

local context = ui.context_inspector
window = context.open({
	pack = pack.new({
		id = "pack-accessibility",
		task_id = "task-accessibility",
		entries = {
			{
				id = "entry-accessibility",
				kind = "file",
				ref = "README.md",
				provenance = { source = "repository", ref = "HEAD" },
				trust = "repository",
				token_estimate = { status = "estimated", tokens = 1 },
				transfer = { eligible = true },
			},
		},
	}),
	on_confirm = function() end,
})
panel(window, { ["]"] = "next", ["t"] = "toggle", ["c"] = "confirm", ["x"] = "cancel", ["h"] = "help" })
assert(context.close(), "context inspector must close after accessibility inspection")

local review = ui.diff_review
window = review.open({
	run = run.new({
		id = "run-accessibility",
		task_id = "task-accessibility",
		provider = { name = "codex", session_id = "native-accessibility" },
		process = { pid = 1, executable = "codex" },
		workspace = { kind = "project", root = "/tmp/gator-accessibility" },
		state = "completed",
		timing = {},
		usage = {},
	}),
	changes = { { path = "README.md", before = "old\n", after = "new\n" } },
})
panel(
	window,
	{ ["]"] = "next", ["c"] = "confirm", ["a"] = "accept", ["r"] = "reject", ["x"] = "cancel", ["h"] = "help" }
)
assert(review.close(), "review must close after accessibility inspection")

local workspaces = ui.workspace_dashboard
window = workspaces.open({
	workspaces = { { id = "workspace-accessibility", kind = "project", root = "/tmp", activity = {} } },
})
panel(window, { ["]"] = "next", ["c"] = "confirm", ["x"] = "cancel", ["h"] = "help" })
assert(workspaces.close(), "workspace dashboard must close after accessibility inspection")

local picker = ui.picker
window = picker.open({ title = "Accessibility", items = { { id = "one", label = "One" } }, on_select = function() end })
panel(window, { ["]"] = "next", ["c"] = "confirm", ["x"] = "cancel", ["h"] = "help" })
assert(picker.close(), "picker must close after accessibility inspection")

local markdown = ui.markdown
window = markdown.open()
panel(window, { ["x"] = "cancel", ["h"] = "help" })
assert(markdown.close(), "markdown must close after accessibility inspection")

gator.setup({ ui = { screen_reader = false } })
window = picker.open({ title = "Visual", items = { { id = "one", label = "One" } }, on_select = function() end })
assert(
	vim.bo[vim.api.nvim_win_get_buf(window)].filetype == "gator-picker",
	"visual mode must preserve the panel filetype"
)
assert(not vim.bo[vim.api.nvim_win_get_buf(window)].modifiable, "visual panels must remain read-only")
assert(picker.close(), "visual picker must close")
gator.setup()
