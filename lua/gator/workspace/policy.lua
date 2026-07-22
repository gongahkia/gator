local overlay = require("gator.policy.overlay")
local redact = require("gator.policy.redact")
local M = {}

local function fail(message)
	error("Gator workspace policy: " .. message, 3)
end

function M.resolve(value)
	if not overlay.is(value) then
		fail("resolve requires a policy overlay")
	end
	for key in pairs(value.rules) do
		if key ~= "workspace" then
			fail("policy rule cannot select workspace behavior: " .. key)
		end
	end
	if value.rules.workspace == "current" then
		return { kind = "project", source = value.provenance }
	end
	if value.rules.workspace == "worktree" then
		return { kind = "worktree", source = value.provenance }
	end
	fail("policy requires workspace current or worktree")
end

local function outcome(state, fields)
	fields = fields or {}
	fields.state = state
	return fields
end

local function approval(value)
	if value == nil then
		return { state = "unavailable", reason = "write approval is unavailable" }
	end
	if type(value) ~= "table" then
		fail("approval must be a table")
	end
	for key in pairs(value) do
		if key ~= "state" and key ~= "reason" then
			fail("approval contains unsupported field: " .. tostring(key))
		end
	end
	if type(value.state) ~= "string" then
		fail("approval.state must be a string")
	end
	local states =
		{ approved = true, granted = true, denied = true, pending = true, unavailable = true, cancelled = true }
	if not states[value.state] then
		fail("approval.state is unavailable")
	end
	if value.reason ~= nil and (type(value.reason) ~= "string" or value.reason == "") then
		fail("approval.reason must be non-empty text")
	end
	return { state = value.state, reason = value.reason and redact.text(value.reason) or nil }
end

local function cancelled(callback)
	if callback == nil then
		return false
	end
	local ok, value = pcall(callback)
	if not ok or type(value) ~= "boolean" then
		return nil
	end
	return value
end

local function preflight(opts)
	if type(opts) ~= "table" then
		fail("preflight requires options")
	end
	for key in pairs(opts) do
		if key ~= "policy" and key ~= "write" and key ~= "approval" and key ~= "cancelled" then
			fail("preflight contains unsupported field: " .. tostring(key))
		end
	end
	if not overlay.is(opts.policy) then
		fail("preflight requires a policy overlay")
	end
	if type(opts.write) ~= "boolean" then
		fail("preflight.write must be boolean")
	end
	if opts.cancelled ~= nil and type(opts.cancelled) ~= "function" then
		fail("preflight.cancelled must be a function")
	end
	local stopped = cancelled(opts.cancelled)
	if stopped == nil then
		return outcome("failed", { failure = "cancellation check failed" })
	end
	if stopped then
		return outcome("cancelled")
	end
	if not opts.write then
		return outcome("completed", { write = false, approval = "not_required" })
	end
	if opts.policy.rules.write_allowed ~= true then
		return outcome("unavailable", { failure = "write policy does not permit this launch" })
	end
	local value = approval(opts.approval)
	if value.state == "approved" or value.state == "granted" then
		return outcome("completed", { write = true, approval = value.state })
	end
	if value.state == "cancelled" then
		return outcome("cancelled", { failure = value.reason or "write approval was cancelled" })
	end
	if value.state == "denied" then
		return outcome("failed", { failure = value.reason or "write approval was denied" })
	end
	return outcome("unavailable", { failure = value.reason or "write approval is " .. value.state })
end

function M.preflight(opts)
	return preflight(opts)
end

function M.launch(opts)
	if type(opts) ~= "table" then
		fail("launch requires options")
	end
	for key in pairs(opts) do
		if key ~= "policy" and key ~= "write" and key ~= "approval" and key ~= "cancelled" and key ~= "launch" then
			fail("launch contains unsupported field: " .. tostring(key))
		end
	end
	if type(opts.launch) ~= "function" then
		fail("launch requires a provider-native launch function")
	end
	local value = preflight({
		policy = opts.policy,
		write = opts.write,
		approval = opts.approval,
		cancelled = opts.cancelled,
	})
	if value.state ~= "completed" then
		return value
	end
	local ok, result = pcall(opts.launch)
	if not ok or result == false then
		return outcome("failed", { failure = "provider-native launch failed", preflight = value })
	end
	return outcome("completed", { preflight = value, launch = result })
end

return M
