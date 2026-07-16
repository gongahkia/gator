local adapters = require("gator").module("adapters")
local handoff = require("gator").module("context").handoff
local supported = { available = true, modes = { "native" } }
local function contract(context, tool)
	return adapters.capabilities.new({
		provider = "codex",
		transport = supported,
		auth = supported,
		session = supported,
		permission = supported,
		model = supported,
		command = supported,
		tool = tool,
		context = context,
		usage = supported,
	})
end
local native =
	handoff.tools({ capabilities = contract({ available = true, modes = { "agent_retrieval" } }, supported) })
assert(
	native.transport == "native" and native.tools[1].name == "gator.search_context",
	"native retrieval providers must receive Gator tools"
)
local mcp = handoff.tools({ capabilities = contract(supported, { available = true, modes = { "mcp" } }) })
assert(mcp.transport == "mcp", "MCP providers must receive Gator tools")
local unavailable = handoff.tools({ capabilities = contract(supported, supported) })
assert(not unavailable.available, "unadvertised retrieval handoff must remain unavailable")
