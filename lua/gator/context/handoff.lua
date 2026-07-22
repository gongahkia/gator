local capabilities = require("gator.adapters.capabilities")
local pack = require("gator.context.handoff_pack")
local redact = require("gator.policy.redact")
local M = {}

local function fail(message)
	error("Gator retrieval handoff: " .. message, 3)
end

local tools = {
	{ name = "gator.search_context", description = "Search Gator's local retrieval index" },
	{ name = "gator.read_context", description = "Read an approved Gator context reference" },
}

function M.tools(opts)
	if type(opts) ~= "table" or not capabilities.is(opts.capabilities) then
		fail("tools requires a capability contract")
	end
	local native = capabilities.supports(opts.capabilities, "context", "agent_retrieval")
	if native then
		return { transport = "native", tools = vim.deepcopy(tools) }
	end
	local mcp = capabilities.supports(opts.capabilities, "tool", "mcp")
	if mcp then
		return { transport = "mcp", tools = vim.deepcopy(tools) }
	end
	return { available = false, reason = "provider does not advertise native retrieval or MCP" }
end

function M.compatibility(opts)
	if type(opts) ~= "table" or not capabilities.is(opts.capabilities) then
		fail("compatibility requires a capability contract")
	end
	local contract = opts.capabilities
	local creates, reason = capabilities.supports(contract, "session", "create")
	if not creates then
		return {
			available = false,
			provider = contract.provider,
			reason = "provider does not advertise session creation: " .. reason,
		}
	end
	local context = M.tools({ capabilities = contract })
	if context.available == false then
		return { available = false, provider = contract.provider, reason = context.reason }
	end
	return {
		available = true,
		provider = contract.provider,
		session = { mode = "create", owner = "provider" },
		transport = context.transport,
		tools = context.tools,
	}
end

local function payload_entry(value)
	return redact.value({
		id = value.id,
		kind = value.kind,
		ref = value.ref,
		provenance = value.provenance,
		trust = value.trust,
		transfer = value.transfer,
		annotation = value.annotation,
		pinned = value.pinned,
		revision = value.revision,
		retrieval_source = value.retrieval_source,
		policy_decision = value.policy_decision,
	})
end

local function approved(record)
	local entries, prompt = {}, nil
	for index, entry in ipairs(record.entries) do
		if not entry.transfer.eligible then
			return nil, "handoff entry is not transfer-eligible: " .. entry.id
		end
		if entry.policy_decision and entry.policy_decision:find("^blocked:") then
			return nil, "handoff entry is blocked by policy: " .. entry.id
		end
		if entry.kind == "task" and type(entry.content) == "string" and entry.content ~= "" then
			prompt = redact.text(entry.content)
		end
		entries[index] = payload_entry(entry)
	end
	if not prompt then
		return nil, "handoff pack has no transferable task prompt"
	end
	return { prompt = prompt, entries = entries }
end

function M.launch_payload(opts)
	if type(opts) ~= "table" then
		fail("launch_payload requires options")
	end
	for key in pairs(opts) do
		if key ~= "pack" and key ~= "capabilities" then
			fail("launch_payload contains unsupported field: " .. tostring(key))
		end
	end
	if not pack.is(opts.pack) then
		fail("launch_payload requires a canonical handoff pack")
	end
	local record = pack.to_record(opts.pack)
	local target = M.compatibility({ capabilities = opts.capabilities })
	if target.available == false then
		return {
			available = false,
			provider = target.provider,
			pack_id = record.id,
			task_id = record.task_id,
			reason = redact.text(target.reason),
		}
	end
	local context, reason = approved(record)
	if not context then
		return {
			available = false,
			provider = target.provider,
			pack_id = record.id,
			task_id = record.task_id,
			reason = redact.text(reason),
		}
	end
	return redact.value({
		available = true,
		kind = "gator.handoff.launch",
		provider = target.provider,
		pack_id = record.id,
		task_id = record.task_id,
		target = {
			session = target.session,
			transport = target.transport,
			tools = target.tools,
		},
		prompt = context.prompt,
		context = { entries = context.entries },
	})
end

return M
