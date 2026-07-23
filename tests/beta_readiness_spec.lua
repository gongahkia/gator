local readiness = require("gator").module("core").beta_readiness
local filesystem = require("gator").module("core").filesystem
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local ready = readiness.verify({
	captured_at = 1,
	checks = {
		{
			name = "compatibility",
			check = function()
				return { state = "ready" }
			end,
		},
		{
			name = "storage",
			check = function()
				return { state = "ready" }
			end,
		},
	},
})
local root = helpers.tempdir("beta-readiness")
local written = readiness.write({ report = ready, root = root, storage = { sharing = "local" } })
assert(
	written.state == "ready"
		and vim.fn.filereadable(written.path) == 1
		and vim.fn.filereadable(root .. "/beta-readiness/failure.json") == 0,
	"ready beta checks must emit a rolling readiness report without a failure bundle"
)
local failed_report = readiness.verify({
	captured_at = 2,
	checks = {
		{
			name = "compatibility",
			check = function()
				return { state = "ready" }
			end,
		},
		{
			name = "storage",
			check = function()
				error("token: private-value")
			end,
		},
	},
})
local failed = readiness.write({ report = failed_report, root = root, storage = { sharing = "local" } })
local failure = helpers.read(failed.failure_path)
assert(
	failed.state == "failed"
		and vim.fn.filereadable(failed.path) == 1
		and vim.fn.filereadable(failed.failure_path) == 1
		and not failure:find("private%-value"),
	"failed beta checks must emit redacted local failure bundles"
)
local cancelled = readiness.verify({
	checks = { {
		name = "compatibility",
		check = function()
			return { state = "ready" }
		end,
	} },
	cancel = function()
		return true
	end,
})
assert(cancelled.state == "cancelled" and readiness.write({
	report = cancelled,
	root = helpers.tempdir("beta-cancelled"),
	storage = { sharing = "local" },
}).state == "cancelled", "beta readiness verification and persistence must support cancellation")
local unavailable = readiness.write({
	report = ready,
	root = helpers.tempdir("beta-unavailable"),
	storage = { sharing = "shared" },
})
assert(unavailable.state == "unavailable", "beta readiness reports must reject non-local durable storage")
local write_failed = readiness.write({
	report = ready,
	root = helpers.tempdir("beta-failed"),
	storage = { sharing = "local" },
	filesystem = filesystem.new({
		mkdir = function()
			error("token: private-value")
		end,
	}),
})
assert(
	write_failed.state == "failed" and not write_failed.reason:find("private%-value"),
	"beta readiness write failures must remain explicit and redacted"
)
assert(
	not pcall(readiness.verify, { checks = {} })
		and not pcall(readiness.write, { report = ready, storage = { sharing = "local" }, extra = true }),
	"beta readiness interfaces must reject invalid input"
)
