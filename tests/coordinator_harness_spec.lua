local root = vim.g.gator_test.root
local harness = dofile(root .. "/tests/coordinator_harness.lua")

local value = harness.new({ settings = { ui = { motion = { enabled = false, interval_ms = 16, reduced = false } } } })
assert(
	harness.is(value) and not value:state().config.ui.motion.enabled,
	"coordinator harness must start an isolated real coordinator with validated settings"
)
local cancelled = 0
local operation = value:operation({
	id = "harness-provider-run",
	key = "harness-provider-run-key",
	kind = "launch",
	cancel = function()
		cancelled = cancelled + 1
	end,
})
assert(
	value:dispatch("cancel_operation", { id = "harness-provider-run", reason = "token: private-value" })
		and operation:status().state == "cancelled"
		and cancelled == 1,
	"coordinator harness must exercise redacted operation cancellation through the public dispatcher"
)
local panel = value:dispatch("runs")
assert(vim.api.nvim_win_is_valid(panel), "coordinator harness must exercise the run graph")
assert(value:cleanup() and not value:cleanup(), "coordinator harness cleanup must be idempotent")
assert(not pcall(value.dispatch, value, "open"), "closed coordinator harnesses must fail explicitly")
assert(not pcall(harness.new, { settings = true }), "coordinator harness must reject invalid fixture settings")
