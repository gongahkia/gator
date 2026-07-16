local overlay = require("gator.policy.overlay")
local M = {}

local function fail(message)
	error("Gator workspace policy: " .. message, 3)
end

function M.resolve(value)
	if not overlay.is(value) then
		fail("resolve requires a policy overlay")
	end
	for key in pairs(value.rules) do
		if key ~= "workspace" then
			fail("policy rule cannot select workspace behavior: " .. key)
		end
	end
	if value.rules.workspace == "current" then
		return { kind = "project", source = value.provenance }
	end
	if value.rules.workspace == "worktree" then
		return { kind = "worktree", source = value.provenance }
	end
	fail("policy requires workspace current or worktree")
end

return M
