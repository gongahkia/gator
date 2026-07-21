local actions = require("gator.ui").session_actions
local adapters = require("gator").module("adapters")
local session = require("gator").module("core").session

local function contract(session_capability)
	local ready = { available = true, modes = { "native" } }
	return adapters.capabilities.new({
		provider = "codex",
		transport = ready,
		auth = ready,
		session = session_capability,
		permission = ready,
		model = ready,
		command = ready,
		tool = ready,
		context = ready,
		usage = ready,
	})
end

local native = session.new({
	task_id = "task-session-actions",
	provider = "codex",
	id = "native-session-actions",
	owner = "provider",
})
local opened, referenced, cancelled
local window = actions.open({
	session = native,
	capabilities = contract({ available = true, modes = { "deep_link" } }),
	native_resolve = function(reference)
		assert(reference.owner == "provider", "native resolution must receive the provider-owned session reference")
		return "codex://thread/native-session-actions"
	end,
	on_open = function(uri, reference)
		opened = { uri = uri, reference = reference }
	end,
	on_reference = function(reference)
		referenced = reference
	end,
	on_cancel = function()
		cancelled = true
	end,
})
assert(vim.api.nvim_win_is_valid(window), "session actions must create a panel")
local content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(
	content:find("Deep link: available · codex://thread/native-session-actions", 1, true)
		and content:find("Open provider-native link", 1, true),
	"session actions must render a validated provider-native deep link"
)
assert(
	actions.activate() and opened.uri == "codex://thread/native-session-actions",
	"deep-link actions must call the supplied opener"
)
assert(
	actions.select(2) == "reference" and actions.activate(),
	"session references must be keyboard-selectable actions"
)
assert(
	referenced.id == "native-session-actions" and referenced.owner == "provider",
	"session references must retain provider ownership"
)
assert(actions.cancel() and cancelled and not actions.inspect(), "session action cancellation must close the panel")

window = actions.open({
	session = native,
	capabilities = contract({ available = false, reason = "provider deep links unavailable" }),
	native_resolve = function()
		error("must not resolve")
	end,
	on_open = function() end,
	on_reference = function() end,
})
content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(
	actions.inspect().link_status == "unavailable"
		and content:find("provider deep links unavailable", 1, true)
		and actions.inspect().actions == 1,
	"unavailable deep links must remain explicit while retaining the session-reference action"
)
assert(actions.close(), "unavailable session action panels must close")
window = actions.open({
	session = native,
	capabilities = contract({ available = true, modes = { "deep_link" } }),
	native_resolve = function()
		return "codex://thread/native-session-actions?token=fixture-secret"
	end,
	on_open = function() end,
	on_reference = function() end,
})
content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(
	actions.inspect().link_status == "failed" and not content:find("fixture-secret", 1, true),
	"credential-bearing deep links must fail through the core resolver without exposing credentials"
)
assert(actions.close(), "failed session action panels must close")
