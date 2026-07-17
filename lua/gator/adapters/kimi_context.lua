local capabilities = require("gator.adapters.capabilities")
local pack = require("gator.context.pack")
local M = {}
local kinds = { file = true, selection = true, diagnostic = true, diff = true }

local function fail(message)
	error("Gator Kimi context: " .. message, 3)
end

function M.submit(opts)
	if
		type(opts) ~= "table"
		or not pack.is(opts.pack)
		or not capabilities.is(opts.capabilities)
		or opts.capabilities.provider ~= "kimi"
		or type(opts.send) ~= "function"
	then
		fail("submit requires a Kimi context pack, capability contract, and send callback")
	end
	local supported, reason = capabilities.supports(opts.capabilities, "context", "embedded")
	if not supported then
		fail("embedded context is unavailable: " .. reason)
	end
	local entries = {}
	for index, entry in ipairs(opts.pack.entries) do
		if not kinds[entry.kind] then
			fail("context entry kind is unsupported by Kimi: " .. entry.kind)
		end
		if not entry.transfer.eligible then
			fail("context entry is not transfer-eligible: " .. entry.id)
		end
		entries[index] = { kind = entry.kind, ref = entry.ref, provenance = vim.deepcopy(entry.provenance) }
	end
	opts.send({ task_id = opts.pack.task_id, entries = entries })
	return #entries
end

return M
