local capabilities = require("gator.adapters.capabilities")
local overlay = require("gator.policy.overlay")
local M = {}

local function fail(message)
	error("Gator Codex policy: " .. message, 3)
end

function M.map(value, contract)
	if not overlay.is(value) or not capabilities.is(contract) or contract.provider ~= "codex" then
		fail("map requires a Codex policy overlay and capability contract")
	end
	local sandbox, sandbox_reason = capabilities.supports(contract, "permission", "sandbox")
	local approval, approval_reason = capabilities.supports(contract, "permission", "approval")
	if not sandbox then
		fail("Codex sandbox mapping is unavailable: " .. sandbox_reason)
	end
	if not approval then
		fail("Codex approval mapping is unavailable: " .. approval_reason)
	end
	for key in pairs(value.rules) do
		if key ~= "write_allowed" then
			fail("policy rule cannot be mapped safely: " .. key)
		end
	end
	if type(value.rules.write_allowed) ~= "boolean" then
		fail("policy requires explicit boolean write_allowed")
	end
	return {
		sandbox_mode = value.rules.write_allowed and "workspace-write" or "read-only",
		approval_policy = "on-request",
	}
end

return M
