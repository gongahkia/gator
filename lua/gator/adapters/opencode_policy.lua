local capabilities = require("gator.adapters.capabilities")
local overlay = require("gator.policy.overlay")
local M = {}

local function fail(message)
	error("Gator OpenCode policy: " .. message, 3)
end

function M.map(value, contract)
	if not overlay.is(value) or not capabilities.is(contract) or contract.provider ~= "opencode" then
		fail("map requires an OpenCode policy overlay and capability contract")
	end
	local available, reason = capabilities.supports(contract, "permission", "mode")
	if not available then
		fail("OpenCode mode mapping is unavailable: " .. reason)
	end
	for key in pairs(value.rules) do
		if key ~= "write_allowed" then
			fail("policy rule cannot be mapped safely: " .. key)
		end
	end
	if type(value.rules.write_allowed) ~= "boolean" then
		fail("policy requires explicit boolean write_allowed")
	end
	return { mode_id = value.rules.write_allowed and "build" or "plan" }
end

return M
