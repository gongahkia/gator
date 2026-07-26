local sdk = require("gator").module("extensions").ui_sdk
local actions = require("gator.ui.actions")
assert(sdk.register({
	name = "fixture-panel",
	render = function(context)
		return { "fixture " .. context.value }
	end,
}) == "fixture-panel", "UI SDK must register panel renderers")
local window = sdk.open("fixture-panel", { value = "panel" })
assert(
	vim.api.nvim_win_is_valid(window)
		and vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false)[1] == "fixture panel",
	"UI SDK must render extension panels without private layout access"
)
assert(sdk.action({
	name = "fixture-ui-action",
	execute = function()
		return "action"
	end,
}) == "fixture-ui-action", "UI SDK must expose workspace actions")
assert(actions.list()[1].name == "fixture-ui-action", "UI SDK actions must be visible in the workspace registry")
assert(actions.unregister("fixture-ui-action"), "UI SDK actions must unregister cleanly")
assert(sdk.unregister("fixture-panel"), "UI SDK panels must unregister cleanly")
assert(not pcall(sdk.open, "fixture-panel"), "unregistered panels must remain unavailable")
