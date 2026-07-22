local adapters = require("gator").module("adapters")
local handoff = require("gator").module("context").handoff
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
local compatible = handoff.compatibility({
	capabilities = contract({ available = true, modes = { "agent_retrieval" } }, supported),
})
assert(
	compatible.available
		and compatible.provider == "codex"
		and compatible.session.owner == "provider"
		and compatible.transport == "native",
	"compatible targets must create a provider-owned fresh session with native retrieval"
)
local compatible_mcp = handoff.compatibility({
	capabilities = contract(supported, { available = true, modes = { "mcp" } }),
})
assert(
	compatible_mcp.available and compatible_mcp.transport == "mcp",
	"compatible targets must resolve the documented MCP fallback"
)
local no_session = handoff.compatibility({
	capabilities = contract(
		{ available = true, modes = { "agent_retrieval" } },
		supported,
		{ available = false, reason = "target does not create sessions" }
	),
})
assert(
	not no_session.available and no_session.reason:find("session creation", 1, true),
	"targets without fresh-session creation must remain unavailable"
)
local no_context = handoff.compatibility({ capabilities = contract(supported, supported) })
assert(
	not no_context.available and no_context.reason:find("retrieval or MCP", 1, true),
	"targets without a documented context transport must remain unavailable"
)
assert(
	not pcall(handoff.compatibility, { capabilities = {} }),
	"target compatibility must reject non-canonical capability records"
)
