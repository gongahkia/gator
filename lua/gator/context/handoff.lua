local capabilities = require("gator.adapters.capabilities")
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

return M
