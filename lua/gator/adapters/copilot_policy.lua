local capabilities = require("gator.adapters.capabilities")
local overlay = require("gator.policy.overlay")
local M = {}

local function fail(message)
	error("Gator Copilot policy: " .. message, 3)
end

function M.map(value, contract)
	if not overlay.is(value) or not capabilities.is(contract) or contract.provider ~= "copilot" then
		fail("map requires a Copilot policy overlay and capability contract")
	end
	local available, reason = capabilities.supports(contract, "permission", "tool_filter")
	if not available then
		fail("Copilot tool-filter mapping is unavailable: " .. reason)
	end
	for key in pairs(value.rules) do
		if key ~= "write_allowed" then
			fail("policy rule cannot be mapped safely: " .. key)
		end
	end
	if type(value.rules.write_allowed) ~= "boolean" then
		fail("policy requires explicit boolean write_allowed")
	end
	if value.rules.write_allowed then
		return {}
	end
	return { available_tools = { "view", "glob", "grep" } }
end

return M
