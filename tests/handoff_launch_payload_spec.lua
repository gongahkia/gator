local adapters = require("gator").module("adapters")
local handoff = require("gator").module("context").handoff
local pack = require("gator").module("context").handoff_pack
local supported = { available = true, modes = { "native" } }

local function contract(context, tool, session)
	return adapters.capabilities.new({
		provider = "codex",
		transport = supported,
		auth = supported,
		session = session or { available = true, modes = { "create" } },
		permission = supported,
		model = supported,
		command = supported,
		tool = tool,
		context = context,
		usage = supported,
	})
end

local source = pack.new({
	id = "handoff-launch",
	task_id = "task-launch",
	entries = {
		{
			id = "task-launch",
			kind = "task",
			ref = "gator-task://task-launch",
			provenance = { source = "task", ref = "task-launch" },
			trust = "manual",
			token_estimate = { status = "estimated", tokens = 1 },
			transfer = { eligible = true },
			content = "Continue token=fixture-secret",
			policy_decision = "allowed by reviewed handoff",
		},
		{
			id = "file-launch",
			kind = "file",
			ref = "file:///tmp/token=fixture-secret",
			provenance = { source = "buffer", ref = "/tmp/project" },
			trust = "repository",
			token_estimate = { status = "estimated", tokens = 1 },
			transfer = { eligible = true },
			content = "raw token=fixture-secret",
			policy_decision = "allowed by repository trust",
		},
	},
})

local payload = handoff.launch_payload({
	pack = source,
	capabilities = contract({ available = true, modes = { "agent_retrieval" } }, supported),
})
local rendered = vim.inspect(payload)
assert(
	payload.available
		and payload.kind == "gator.handoff.launch"
		and payload.provider == "codex"
		and payload.target.session.owner == "provider"
		and payload.target.transport == "native"
		and payload.context.entries[2].content == nil
		and not rendered:find("fixture-secret", 1, true),
	"launch payloads must use fresh provider-owned targets and redact approved references without raw entry content"
)
payload.context.entries[2].ref = "mutated"
assert(source.entries[2].ref ~= "mutated", "launch payloads must not mutate approved handoff packs")

local mcp = handoff.launch_payload({
	pack = source,
	capabilities = contract(supported, { available = true, modes = { "mcp" } }),
})
assert(mcp.available and mcp.target.transport == "mcp", "launch payloads must preserve the documented MCP fallback")

local unavailable = handoff.launch_payload({
	pack = source,
	capabilities = contract(
		{ available = true, modes = { "agent_retrieval" } },
		supported,
		{ available = false, reason = "target cannot create sessions" }
	),
})
assert(
	not unavailable.available and unavailable.reason:find("session creation", 1, true),
	"launch payloads must remain unavailable before a target can create a fresh session"
)

local blocked = pack.new({
	id = "handoff-blocked",
	task_id = "task-launch",
	entries = vim.tbl_extend("force", {}, source.entries),
})
blocked.entries[2].policy_decision = "blocked: repository trust does not allow manual context"
local policy = handoff.launch_payload({
	pack = blocked,
	capabilities = contract({ available = true, modes = { "agent_retrieval" } }, supported),
})
assert(
	not policy.available and policy.reason:find("blocked by policy", 1, true),
	"launch payloads must reject known blocked entries before transfer"
)
assert(
	not pcall(handoff.launch_payload, { pack = {}, capabilities = {} }),
	"launch payloads must reject invalid approved packs and capability records"
)
