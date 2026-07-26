local gator = require("gator").setup()
local coordinator = require("gator.coordinator")

assert(
	vim.deep_equal(coordinator.actions(), {
		"open",
		"health",
		"export_diagnostics",
		"verify_beta_readiness",
		"close",
		"cancel_operation",
		"capture_selection",
		"stop_session",
	}),
	"coordinator must expose supported action names"
)
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
assert(not pcall(gator.dispatch, "palette"), "dispatcher must reject removed palette actions")
assert(not pcall(gator.dispatch, "capture_selection", {}), "dispatcher must validate capture action input")
