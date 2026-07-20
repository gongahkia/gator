local gator = require("gator").setup()
local coordinator = require("gator.coordinator")
local palette = require("gator.ui").palette

local invoked = {}
palette.register({
	kind = "action",
	name = "dispatch-fixture",
	execute = function()
		table.insert(invoked, "palette")
		return "complete"
	end,
})

assert(
	vim.deep_equal(coordinator.actions(), { "open", "health", "close", "capture_selection", "palette" }),
	"coordinator must expose supported action names"
)
assert(
	gator.dispatch("palette", { id = "action:dispatch-fixture" }) == "complete",
	"dispatcher must route palette actions"
)
assert(invoked[1] == "palette", "dispatcher must preserve action results")

local window = gator.dispatch("open")
assert(vim.api.nvim_win_is_valid(window), "dispatcher must route workspace opening")
assert(gator.dispatch("close"), "dispatcher must expose explicit workspace cancellation")
assert(not pcall(gator.dispatch, "missing"), "dispatcher must reject unavailable actions")
assert(
	not pcall(gator.dispatch, "palette", { id = "action:dispatch-fixture", extra = true }),
	"dispatcher must reject unsupported options"
)
assert(not pcall(gator.dispatch, "capture_selection", {}), "dispatcher must validate capture action input")
assert(palette.unregister("action:dispatch-fixture"), "dispatch fixtures must clean up registered palette actions")
