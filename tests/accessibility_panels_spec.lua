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
local adapters = require("gator").module("adapters")
local approval_event = require("gator").module("core").approval_event
local provider_event = require("gator").module("core").provider_event
local session = require("gator").module("core").session
local usage_event = require("gator").module("core").usage_event

local function contract(provider, overrides)
	local ready = { available = true, modes = { "native" } }
	local attrs = {
		provider = provider,
		transport = ready,
		auth = ready,
		session = ready,
		permission = ready,
		model = ready,
		command = ready,
		tool = ready,
		context = ready,
		usage = ready,
	}
	for key, value in pairs(overrides or {}) do
		attrs[key] = value
	end
	return adapters.capabilities.new(attrs)
end

local function event(id, sequence, event_type, payload)
	return provider_event.new({
		schema_version = provider_event.schema_version,
		id = id,
		run_id = "run-accessibility",
		provider = { name = "codex", session_id = "native-accessibility" },
		sequence = sequence,
		type = event_type,
		at = sequence,
		payload = payload,
	})
end

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

local event_details = ui.event_details
local event_cancelled = false
window = event_details.open({
	events = {
		event("event-message-accessibility", 1, "message.delta", { text = "message" }),
		event("event-thought-accessibility", 2, "message.thought", { summary = "reasoning" }),
	},
	on_cancel = function()
		event_cancelled = true
	end,
})
panel(window, { ["]"] = "next", ["["] = "previous", ["x"] = "cancel", ["h"] = "help" })
press("]")
assert(event_details.inspect().selected == 2, "event details must navigate with the configured next key")
press("x")
assert(event_cancelled and not event_details.inspect(), "event details must cancel with the configured key")

local approval_details = ui.approval_details
local approval_decision
window = approval_details.open({
	request = approval_event.request({
		id = "approval-accessibility",
		run_id = "run-accessibility",
		provider = { name = "codex", session_id = "native-accessibility" },
		sequence = 3,
		at = 3,
		request_id = "request-accessibility",
		action = "write file",
	}),
	on_decide = function(value)
		approval_decision = value.decision
	end,
})
panel(window, { ["a"] = "accept", ["r"] = "reject", ["x"] = "cancel", ["h"] = "help" })
press("r")
assert(
	approval_decision == "denied" and not approval_details.inspect(),
	"approval details must deny with the configured reject key"
)

local usage_details = ui.usage_details
local usage_cancelled = false
window = usage_details.open({
	events = {
		usage_event.usage({
			id = "usage-accessibility",
			run_id = "run-accessibility",
			provider = { name = "codex", session_id = "native-accessibility" },
			sequence = 4,
			at = 4,
			input_tokens = 1,
			output_tokens = 2,
			total_tokens = 3,
		}),
		usage_event.compaction({
			id = "compaction-accessibility",
			run_id = "run-accessibility",
			provider = { name = "codex", session_id = "native-accessibility" },
			sequence = 5,
			at = 5,
			before_tokens = 8,
			after_tokens = 4,
			summary = "compacted",
		}),
	},
	on_cancel = function()
		usage_cancelled = true
	end,
})
panel(window, { ["]"] = "next", ["["] = "previous", ["x"] = "cancel", ["h"] = "help" })
press("]")
assert(usage_details.inspect().selected == 2, "usage details must navigate with the configured next key")
press("x")
assert(usage_cancelled and not usage_details.inspect(), "usage details must cancel with the configured key")

local session_actions = ui.session_actions
local opened_link
local session_cancelled = false
window = session_actions.open({
	session = session.new({
		task_id = "task-accessibility",
		provider = "codex",
		id = "native-accessibility",
		owner = "provider",
	}),
	capabilities = contract("codex", { session = { available = true, modes = { "deep_link" } } }),
	native_resolve = function()
		return "codex://thread/native-accessibility"
	end,
	on_open = function(uri)
		opened_link = uri
	end,
	on_reference = function() end,
	on_cancel = function()
		session_cancelled = true
	end,
})
panel(window, { ["]"] = "next", ["["] = "previous", ["c"] = "confirm", ["x"] = "cancel", ["h"] = "help" })
press("c")
assert(opened_link == "codex://thread/native-accessibility", "session actions must confirm with the configured key")
press("x")
assert(session_cancelled and not session_actions.inspect(), "session actions must cancel with the configured key")

local handoff_review = ui.handoff_review
local handoff
window = handoff_review.open({
	mode = "manual",
	source_provider = "codex",
	target_provider = "claude",
	content = "initial summary",
	target_capabilities = contract("claude", {
		session = { available = true, modes = { "create" } },
		context = { available = true, modes = { "agent_retrieval" } },
		tool = { available = true, modes = { "native" } },
	}),
	opt_in = false,
	on_confirm = function(summary)
		handoff = summary
	end,
})
panel(window, { ["c"] = "confirm", ["p"] = "prompt", ["x"] = "cancel", ["h"] = "help" })
local input = vim.ui.input
vim.ui.input = function(_, callback)
	callback("edited summary")
end
press("p")
vim.ui.input = input
assert(
	table
		.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
		:find("edited summary", 1, true),
	"handoff review must edit with the configured prompt key"
)
press("c")
assert(
	handoff and handoff.content == "edited summary" and not handoff_review.inspect(),
	"handoff review must confirm with the configured key"
)

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
