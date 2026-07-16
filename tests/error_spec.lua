local errors = require("gator").error
local value = errors.new("capability.missing", "Codex CLI is unavailable", {
	detail = "codex was not found on PATH",
	remedy = "Install Codex CLI or configure its executable path.",
})

assert(errors.is(value), "new Gator errors must be typed")
assert(
	errors.format(value)
		== "[capability.missing] Codex CLI is unavailable\nDetails: codex was not found on PATH\nRecovery: Install Codex CLI or configure its executable path.",
	"formatted errors must include details and recovery"
)

local notification
local original_notify = vim.notify
vim.notify = function(message, level, options)
	notification = { message = message, level = level, options = options }
end
errors.notify(value)
vim.notify = original_notify

assert(notification.message == errors.format(value), "notifications must preserve formatted errors")
assert(notification.level == vim.log.levels.ERROR, "notifications must default to error level")
assert(notification.options.title == "Gator", "notifications must identify Gator")

local ok = pcall(errors.new, "Capability Missing", "message", { remedy = "retry" })
assert(not ok, "invalid error codes must fail explicitly")
ok = pcall(errors.new, "capability.missing", "message", {})
assert(not ok, "errors without recovery guidance must fail explicitly")
ok = pcall(errors.notify, "untyped")
assert(not ok, "untyped notifications must fail explicitly")
local raised
ok, raised = pcall(errors.raise, value)
assert(not ok and raised:find("Recovery:", 1, true), "raised errors must preserve recovery guidance")
