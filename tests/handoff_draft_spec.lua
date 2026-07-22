local adapters = require("gator").module("adapters")
local draft = require("gator").module("context").handoff_draft
local evidence = require("gator").module("context").handoff_evidence
local handoff_pack = require("gator").module("context").handoff_pack
local pack = require("gator").module("context").pack

local function contract(session)
	local ready = { available = true, modes = { "native" } }
	return adapters.capabilities.new({
		provider = "codex",
		transport = ready,
		auth = ready,
		session = session,
		permission = ready,
		model = ready,
		command = ready,
		tool = ready,
		context = ready,
		usage = ready,
	})
end

local source_evidence = evidence.new({
	task_id = "task-draft",
	state = "ready",
	source = {
		provider = "codex",
		run_id = "run-source",
		session = { provider = "codex", id = "native-source", owner = "provider" },
	},
	decisions = {
		{ event_id = "event-decision", type = "message.thought", at = 1, summary = "inspect token=fixture-secret" },
	},
	outcomes = { { event_id = "event-outcome", type = "message.completed", at = 2, summary = "completed" } },
})
local source_pack = handoff_pack.new({
	id = "pack-draft",
	task_id = "task-draft",
	entries = {
		pack.entry({
			id = "entry-draft",
			kind = "file",
			ref = "file://README.md",
			provenance = { source = "fixture", ref = "README.md" },
			trust = "manual",
			token_estimate = { status = "estimated", tokens = 1 },
			transfer = { eligible = true },
			content = "token=fixture-secret",
		}),
	},
})

local sent, complete
local request = draft.new({
	id = "draft-source",
	pack = source_pack,
	evidence = source_evidence,
	capabilities = contract({ available = true, modes = { "resume" } }),
	send = function(payload, callback)
		sent, complete = payload, callback
		return true
	end,
})
assert(request:status().state == "ready", "source handoff drafts must begin ready after capability preflight")
assert(
	request:dispatch() and request:status().state == "pending",
	"draft dispatch must remain non-blocking until source completion"
)
assert(
	sent.source_session.owner == "provider"
		and sent.source_session.id == "native-source"
		and sent.context.entries[1].content == nil
		and vim.json.encode(sent):find("fixture-secret", 1, true) == nil,
	"draft requests must preserve provider-owned source sessions without transferring raw context content or secrets"
)
complete("summary token=fixture-secret")
assert(
	request:status().state == "completed" and request:status().content:find("fixture-secret", 1, true) == nil,
	"source draft completion must redact the returned summary"
)

local cancelled = draft.new({
	id = "draft-cancelled",
	pack = source_pack,
	evidence = source_evidence,
	capabilities = contract({ available = true, modes = { "resume" } }),
	send = function()
		error("must not send")
	end,
})
assert(
	cancelled:cancel() and not cancelled:dispatch() and cancelled:status().state == "cancelled",
	"draft requests must cancel before dispatch"
)

local failed = draft.new({
	id = "draft-failed",
	pack = source_pack,
	evidence = source_evidence,
	capabilities = contract({ available = true, modes = { "resume" } }),
	send = function()
		error("token=fixture-secret")
	end,
})
assert(
	not failed:dispatch()
		and failed:status().state == "failed"
		and failed:status().reason:find("fixture-secret", 1, true) == nil,
	"source draft failures must remain explicit and redacted"
)

local unavailable = draft.new({
	id = "draft-unavailable",
	pack = source_pack,
	evidence = source_evidence,
	capabilities = contract({ available = false, reason = "provider resume unavailable" }),
	send = function()
		error("must not send")
	end,
})
assert(
	unavailable:status().state == "unavailable"
		and unavailable:status().reason:find("provider resume unavailable", 1, true),
	"source drafts must expose unavailable provider-native resume capability"
)
assert(not pcall(draft.new, {
	id = "draft-invalid",
	pack = handoff_pack.new({ id = "pack-other", task_id = "task-other", entries = {} }),
	evidence = source_evidence,
	capabilities = contract({ available = true, modes = { "resume" } }),
	send = function() end,
}), "source drafts must reject handoff packs from another task")
