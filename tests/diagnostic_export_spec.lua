local config = require("gator.config")
local diagnostic_export = require("gator").module("core").diagnostic_export
local filesystem = require("gator").module("core").filesystem
local state = require("gator.state")
local compat = require("gator.compat")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local value = state.new(config.resolve(), compat.inspect())
value:update({
	tasks = { { objective = "token: private-value" } },
	context = { selections = { { path = "/private/path" } } },
	workspace = { status = "degraded", detail = "token: private-value /private/path" },
})
local root = helpers.tempdir("diagnostic-export")
local ready = diagnostic_export.write({
	state = value,
	root = root,
	storage = { sharing = "local" },
	captured_at = 1,
})
local stored = vim.json.decode(helpers.read(ready.path))
assert(
	ready.state == "ready"
		and ready.path == root .. "/diagnostics/current.json"
		and stored.schema_version == 1
		and stored.captured_at == 1
		and stored.network_telemetry == false
		and stored.retention.mode == "rolling"
		and stored.retention.files == 1,
	"diagnostic exports must write one versioned local-only rolling report"
)
local encoded = helpers.read(ready.path)
assert(
	not encoded:find("private%-value")
		and not encoded:find("/private/path", 1, true)
		and stored.workspace.status == "degraded"
		and stored.configuration.persistence.sharing == "local",
	"diagnostic exports must exclude task, context, path, and credential data while retaining safe local state"
)
local cancelled = diagnostic_export.write({
	state = value,
	root = helpers.tempdir("diagnostic-cancelled"),
	storage = { sharing = "local" },
	cancel = function()
		return true
	end,
})
assert(
	cancelled.state == "cancelled" and vim.fn.filereadable(cancelled.path) == 0,
	"diagnostic exports must support cancellation before durable writes"
)
local unavailable = diagnostic_export.write({
	state = value,
	root = helpers.tempdir("diagnostic-unavailable"),
	storage = { sharing = "shared" },
})
assert(
	unavailable.state == "unavailable" and vim.fn.filereadable(unavailable.path) == 0,
	"diagnostic exports must reject non-local durable storage"
)
local failed = diagnostic_export.write({
	state = value,
	root = helpers.tempdir("diagnostic-failed"),
	storage = { sharing = "local" },
	filesystem = filesystem.new({
		mkdir = function()
			error("token: private-value")
		end,
	}),
})
assert(
	failed.state == "failed" and not failed.reason:find("private%-value"),
	"diagnostic export failures must remain explicit and redacted"
)
assert(
	not pcall(diagnostic_export.write, { state = value, root = root, storage = { sharing = "local" }, extra = true }),
	"diagnostic exports must reject unsupported options"
)
