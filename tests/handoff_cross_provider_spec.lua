local adapters = require("gator").module("adapters")
local evidence = require("gator").module("context").handoff_evidence
local handoff = require("gator").module("context").handoff
local lineage = require("gator").module("context").handoff_lineage
local pack = require("gator").module("context").handoff_pack
local target = require("gator").module("context").handoff_target

local providers = {
	"aider",
	"amp",
	"cline",
	"cursor",
	"codex",
	"claude",
	"gemini",
	"goose",
	"kimi",
	"vibe",
	"copilot",
	"opencode",
	"pi",
}

local supported = { available = true, modes = { "native" } }

local function contract(provider)
	return adapters.capabilities.new({
		provider = provider,
		transport = supported,
		auth = { available = false, reason = "provider-owned authentication" },
		session = { available = true, modes = { "create" } },
		permission = supported,
		model = supported,
		command = supported,
		tool = { available = true, modes = { "mcp" } },
		context = { available = true, modes = { "agent_retrieval" } },
		usage = supported,
	})
end

local pairs = 0
for _, source in ipairs(providers) do
	for _, destination in ipairs(providers) do
		if source ~= destination then
			pairs = pairs + 1
			local id = source .. "-to-" .. destination
			local task_id = "task-" .. id
			local source_pack = pack.new({
				id = "pack-" .. id,
				task_id = task_id,
				entries = {
					{
						id = "task-" .. id,
						kind = "task",
						ref = "gator-task://" .. task_id,
						provenance = { source = "task", ref = task_id },
						trust = "manual",
						token_estimate = { status = "estimated", tokens = 1 },
						transfer = { eligible = true },
						content = "Continue the approved handoff from " .. source .. " to " .. destination .. ".",
						policy_decision = "allowed by protected handoff review",
					},
				},
			})
			local payload = handoff.launch_payload({ pack = source_pack, capabilities = contract(destination) })
			assert(
				payload.available
					and payload.provider == destination
					and payload.target.session.owner == "provider"
					and not vim.inspect(payload):find("source-session", 1, true),
				"cross-provider handoffs must create a fresh target payload without source-session transfer: " .. id
			)
			local request = target.new({
				id = "request-" .. id,
				payload = payload,
				create = function(value)
					assert(
						value.provider == destination and value.source_session == nil,
						"cross-provider target creation must retain target ownership: " .. id
					)
					return { provider = destination, id = "target-session-" .. id, owner = "provider" }
				end,
			})
			assert(request:dispatch(), "cross-provider target creation must dispatch: " .. id)
			local source_evidence = evidence.from_record({
				schema_version = 1,
				task_id = task_id,
				state = "ready",
				source = {
					provider = source,
					run_id = "source-run-" .. id,
					session = { provider = source, id = "source-session-" .. id, owner = "provider" },
				},
				decisions = { { event_id = "decision-" .. id, type = "message.thought", at = 1, summary = "approved" } },
				outcomes = {},
			})
			local record = lineage.new({
				id = "lineage-" .. id,
				pack = source_pack,
				evidence = source_evidence,
				target = request:session(),
				at = 1,
			})
			assert(
				record.source.provider == source and record.target.provider == destination,
				"cross-provider lineage must preserve distinct source and target ownership: " .. id
			)
		end
	end
end

assert(
	pairs == #providers * (#providers - 1),
	"cross-provider handoff coverage must include every directed provider pair"
)
