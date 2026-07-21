local control = require("gator").module("adapters").opencode_control
local calls = {}
local native = control.new({
	session = { provider = "opencode", id = "opencode-fixture", owner = "provider" },
	cwd = vim.g.gator_test.root,
	notify = function(value)
		table.insert(calls, value)
	end,
	request = function(value)
		table.insert(calls, value)
		return { id = value.id, result = { configOptions = {} } }
	end,
})
assert(
	native:cancel("token=fixture-secret").state == "cancelled",
	"OpenCode cancellation must use native ACP session/cancel"
)
assert(
	calls[1].method == "session/cancel"
		and calls[1].params.sessionId == "opencode-fixture"
		and native:recover().session.owner == "provider"
		and calls[2].method == "session/load"
		and calls[2].params.sessionId == "opencode-fixture"
		and calls[2].params.cwd == vim.uv.fs_realpath(vim.g.gator_test.root),
	"OpenCode recovery must load the provider-owned session in its original workspace"
)

local failed = control.new({
	session = { provider = "opencode", id = "opencode-fixture", owner = "provider" },
	cwd = vim.g.gator_test.root,
	notify = function()
		error("token=fixture-secret", 0)
	end,
	request = function() end,
})
assert(
	failed:cancel().state == "failed" and failed:status().reason == "token=[REDACTED]",
	"OpenCode cancellation failures must remain explicit and redacted"
)

local invalid = control.new({
	session = { provider = "opencode", id = "opencode-fixture", owner = "provider" },
	cwd = vim.g.gator_test.root,
	notify = function() end,
	request = function(value)
		return { id = value.id, result = { sessionId = "other-session" } }
	end,
})
assert(invalid:recover().state == "failed", "OpenCode recovery must reject unsupported recovery data")

local unavailable = control.unavailable("token=fixture-secret")
assert(
	unavailable:recover().state == "unavailable" and unavailable:status().reason == "token=[REDACTED]",
	"OpenCode control unavailability must remain explicit and redacted"
)
