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

return M
