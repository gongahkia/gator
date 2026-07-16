local capabilities = require("gator.adapters.capabilities")
local pack = require("gator.context.pack")
local M = {}
local kinds = { file = true, selection = true, diagnostic = true, diff = true }

local function fail(message)
	error("Gator OpenCode context: " .. message, 3)
end

local function require_capability(contract, domain, mode)
	local supported, reason = capabilities.supports(contract, domain, mode)
	if not supported then
		fail(domain .. " capability is unavailable: " .. reason)
	end
end

function M.submit(opts)
	if
		type(opts) ~= "table"
		or not pack.is(opts.pack)
		or not capabilities.is(opts.capabilities)
		or opts.capabilities.provider ~= "opencode"
		or type(opts.send) ~= "function"
	then
		fail("submit requires an OpenCode context pack, capability contract, and send callback")
	end
	require_capability(opts.capabilities, "context", "attach")
	local entries = {}
	for index, entry in ipairs(opts.pack.entries) do
		if not kinds[entry.kind] then
			fail("context entry kind is unsupported by OpenCode: " .. entry.kind)
		end
		if not entry.transfer.eligible then
			fail("context entry is not transfer-eligible: " .. entry.id)
		end
		entries[index] = { kind = entry.kind, ref = entry.ref, provenance = vim.deepcopy(entry.provenance) }
	end
	opts.send({ task_id = opts.pack.task_id, entries = entries })
	return #entries
end

function M.command(opts)
	if
		type(opts) ~= "table"
		or not capabilities.is(opts.capabilities)
		or opts.capabilities.provider ~= "opencode"
		or type(opts.execute) ~= "function"
	then
		fail("command requires an OpenCode capability contract and execute callback")
	end
	require_capability(opts.capabilities, "command", "execute")
	if
		type(opts.name) ~= "string"
		or opts.name == ""
		or type(opts.advertised) ~= "table"
		or not vim.islist(opts.advertised)
	then
		fail("command requires a name and advertised command list")
	end
	if not vim.tbl_contains(opts.advertised, opts.name) then
		fail("OpenCode command is not advertised: " .. opts.name)
	end
	opts.execute(opts.name)
end

return M
