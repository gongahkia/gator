local capabilities = require("gator.adapters.capabilities")
local pack = require("gator.context.pack")
local M = {}

local function fail(message)
	error("Gator Aider context: " .. message, 3)
end

function M.submit(opts)
	if
		type(opts) ~= "table"
		or not pack.is(opts.pack)
		or not capabilities.is(opts.capabilities)
		or opts.capabilities.provider ~= "aider"
		or type(opts.send) ~= "function"
	then
		fail("submit requires an Aider context pack, capability contract, and send callback")
	end
	local supported, reason = capabilities.supports(opts.capabilities, "context", "read")
	if not supported then
		fail("context read is unavailable: " .. reason)
	end
	local paths = {}
	for index, entry in ipairs(opts.pack.entries) do
		if entry.kind ~= "file" then
			fail("context entry kind is unsupported by Aider: " .. entry.kind)
		end
		if not entry.transfer.eligible then
			fail("context entry is not transfer-eligible: " .. entry.id)
		end
		if type(entry.ref) ~= "string" or entry.ref == "" then
			fail("context file ref must be non-empty: " .. entry.id)
		end
		paths[index] = entry.ref
	end
	opts.send({ read = paths })
	return #paths
end

return M
