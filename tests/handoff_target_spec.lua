local adapters = require("gator").module("adapters")
local handoff = require("gator").module("context").handoff
local target = require("gator").module("context").handoff_target
local pack = require("gator").module("context").handoff_pack
local supported = { available = true, modes = { "native" } }

local function contract(session)
	return adapters.capabilities.new({
		provider = "codex",
		transport = supported,
		auth = supported,
		session = session or { available = true, modes = { "create" } },
		permission = supported,
		model = supported,
		command = supported,
		tool = supported,
		context = { available = true, modes = { "agent_retrieval" } },
		usage = supported,
	})
end

local source = pack.new({
	id = "handoff-target",
	task_id = "task-target",
	entries = {
		{
			id = "task-target",
			kind = "task",
			ref = "gator-task://task-target",
			provenance = { source = "task", ref = "task-target" },
			trust = "manual",
			token_estimate = { status = "estimated", tokens = 1 },
			transfer = { eligible = true },
			content = "Continue the approved handoff",
		},
	},
})
local payload = handoff.launch_payload({ pack = source, capabilities = contract() })
local received
local request = target.new({
	id = "target-request",
	payload = payload,
	create = function(value)
		received = value
		return { provider = "codex", id = "target-native", owner = "provider" }
	end,
})
assert(request:status().state == "ready" and request:dispatch(), "target handoffs must begin ready and dispatch once")
local created = request:session()
assert(
	request:status().state == "completed"
		and created.id == "target-native"
		and created.task_id == "task-target"
		and created.owner == "provider"
		and received.target.session.mode == "create"
		and received.target.session.id == nil
		and received.source_session == nil,
	"target handoffs must create a fresh provider-owned session without source-session transfer"
)
assert(not request:cancel(), "completed target handoffs must not cancel")

local unavailable = target.new({
	id = "target-unavailable",
	payload = handoff.launch_payload({
		pack = source,
		capabilities = contract({ available = false, reason = "target cannot create sessions" }),
	}),
	create = function()
		error("must not create")
	end,
})
assert(
	unavailable:status().state == "unavailable" and not unavailable:dispatch(),
	"unavailable target payloads must not invoke a provider session creator"
)

local failed = target.new({
	id = "target-failed",
	payload = payload,
	create = function()
		return false, "token=fixture-secret"
	end,
})
assert(
	not failed:dispatch()
		and failed:status().state == "failed"
		and not failed:status().reason:find("fixture-secret", 1, true),
	"target creation failures must remain explicit and redacted"
)

local pending = target.new({
	id = "target-cancelled",
	payload = payload,
	create = function()
		return true
	end,
})
assert(
	pending:dispatch()
		and pending:status().state == "pending"
		and pending:cancel("token=fixture-secret")
		and pending:status().state == "cancelled"
		and not pending:complete({ provider = "codex", id = "late", owner = "provider" }),
	"pending target creation must cancel once and reject late provider completion"
)

local forged = vim.deepcopy(payload)
forged.source_session = { provider = "codex", id = "source-session", owner = "provider" }
assert(
	not pcall(target.new, { id = "target-forged", payload = forged, create = function() end }),
	"target handoffs must reject payloads that carry source-session data"
)
local mismatch = target.new({
	id = "target-mismatch",
	payload = payload,
	create = function()
		return { provider = "gemini", id = "wrong-provider", owner = "provider" }
	end,
})
assert(
	not mismatch:dispatch() and mismatch:status().state == "failed",
	"target handoffs must reject sessions owned by another provider"
)

local callbacks, retry_calls = {}, 0
local retried = target.new({
	id = "target-retry",
	payload = payload,
	create = function(_, complete)
		retry_calls = retry_calls + 1
		callbacks[retry_calls] = complete
		return true
	end,
})
assert(
	retried:dispatch() and retried:cancel("token=fixture-secret"),
	"pending target handoffs must record cancellation"
)
local cancelled = retried:outcome()
assert(
	cancelled.state == "cancelled"
		and cancelled.attempts == 1
		and cancelled.retries == 0
		and not cancelled.reason:find("fixture-secret", 1, true),
	"cancelled handoffs must expose a redacted terminal outcome"
)
assert(retried:retry() and retried:dispatch() and retry_calls == 2, "cancelled handoffs must retry with a new attempt")
assert(
	not callbacks[1]({ provider = "codex", id = "stale", owner = "provider" }) and retried:status().state == "pending",
	"late completion from a cancelled attempt must not settle a retry"
)
assert(
	callbacks[2]({ provider = "codex", id = "retried-target", owner = "provider" }),
	"current retry completion must settle"
)
local completed = retried:outcome()
assert(
	completed.state == "completed"
		and completed.attempts == 2
		and completed.retries == 1
		and completed.session.id == "retried-target"
		and #retried:outcomes() == 2,
	"retries must retain ordered cancellation and completion outcomes"
)
assert(
	not retried:retry() and not retried:cancel(),
	"completed handoffs must retain a terminal outcome without further retry or cancellation"
)
