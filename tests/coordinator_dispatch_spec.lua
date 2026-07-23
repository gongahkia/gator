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
	vim.deep_equal(coordinator.actions(), {
		"open",
		"health",
		"export_diagnostics",
		"verify_beta_readiness",
		"close",
		"cancel_operation",
		"capture_selection",
		"palette",
	}),
	"coordinator must expose supported action names"
)
assert(
	gator.dispatch("palette", { id = "action:dispatch-fixture" }) == "complete",
	"dispatcher must route palette actions"
)
assert(invoked[1] == "palette", "dispatcher must preserve action results")
local diagnostic = gator.export_diagnostics()
assert(
	diagnostic.state == "ready"
		and vim.fn.filereadable(diagnostic.path) == 1
		and vim.json.decode(table.concat(vim.fn.readfile(diagnostic.path), "\n")).network_telemetry == false,
	"dispatcher must write local-only diagnostic exports"
)
local readiness = gator.verify_beta_readiness()
assert(
	readiness.state == "ready"
		and vim.fn.filereadable(readiness.path) == 1
		and vim.json.decode(table.concat(vim.fn.readfile(readiness.path), "\n")).state == "ready",
	"dispatcher must write public-beta readiness reports"
)

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
