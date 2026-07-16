local sdk = require("gator").module("extensions").ui_sdk
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
}) == "action:fixture-ui-action", "UI SDK must expose extension actions")
assert(sdk.unregister("fixture-panel"), "UI SDK panels must unregister cleanly")
assert(not pcall(sdk.open, "fixture-panel"), "unregistered panels must remain unavailable")
