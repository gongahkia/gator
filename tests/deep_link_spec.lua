local adapters = require("gator").module("adapters")
local links = require("gator").module("core").deep_link
local session = require("gator").module("core").session

local function contract(session_capability)
	local supported = { available = true, modes = { "native" } }
	return adapters.capabilities.new({
		provider = "codex",
		transport = supported,
		auth = supported,
		session = session_capability,
		permission = supported,
		model = supported,
		command = supported,
		tool = supported,
		context = supported,
		usage = supported,
	})
end

local native = session.new({ task_id = "task-link", provider = "codex", id = "native-link", owner = "provider" })
local resolved
local available = links.resolve({
	session = native,
	capabilities = contract({ available = true, modes = { "deep_link" } }),
	native_resolve = function(reference)
		resolved = reference
		return "codex://thread/native-link"
	end,
})
assert(
	available.status == "available"
		and available.uri == "codex://thread/native-link"
		and resolved.owner == "provider"
		and resolved.id == "native-link",
	"deep links must resolve provider-owned native session references"
)

local unavailable = links.resolve({
	session = native,
	capabilities = contract({ available = false, reason = "native links unavailable" }),
	native_resolve = function()
		error("must not resolve")
	end,
})
assert(
	unavailable.status == "unavailable" and unavailable.reason == "native links unavailable",
	"unavailable provider links must remain explicit"
)

local cancelled = links.resolve({
	session = native,
	capabilities = contract({ available = true, modes = { "deep_link" } }),
	cancelled = function()
		return true
	end,
	native_resolve = function()
		error("must not resolve")
	end,
})
assert(cancelled.status == "cancelled", "deep-link resolution must honour cancellation before external access")

local is_cancelled = false
local stopped = links.resolve({
	session = native,
	capabilities = contract({ available = true, modes = { "deep_link" } }),
	cancelled = function()
		return is_cancelled
	end,
	native_resolve = function()
		is_cancelled = true
		return "codex://thread/native-link"
	end,
})
assert(stopped.status == "cancelled", "deep-link resolution must discard references after cancellation")

local failed = links.resolve({
	session = native,
	capabilities = contract({ available = true, modes = { "deep_link" } }),
	native_resolve = function()
		error("token: private-value")
	end,
})
assert(
	failed.status == "failed" and not failed.reason:find("private%-value"),
	"provider deep-link failures must be explicit and redacted"
)

local unsafe = links.resolve({
	session = native,
	capabilities = contract({ available = true, modes = { "deep_link" } }),
	native_resolve = function()
		return "codex://thread/native-link?token=private-value"
	end,
})
assert(unsafe.status == "failed", "deep links must reject credential-bearing URIs")
