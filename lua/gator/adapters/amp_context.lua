local capabilities = require("gator.adapters.capabilities")
local pack = require("gator.context.pack")
local M = {}
local kinds = { file = true, selection = true, diagnostic = true, diff = true }

local function fail(message)
	error("Gator Amp context: " .. message, 3)
end

function M.submit(opts)
	if
		type(opts) ~= "table"
		or not pack.is(opts.pack)
		or not capabilities.is(opts.capabilities)
		or opts.capabilities.provider ~= "amp"
		or type(opts.send) ~= "function"
	then
		fail("submit requires an Amp context pack, capability contract, and send callback")
	end
	local supported, reason = capabilities.supports(opts.capabilities, "context", "input")
	if not supported then
		fail("context input is unavailable: " .. reason)
	end
	local sections = {}
	for index, entry in ipairs(opts.pack.entries) do
		if not kinds[entry.kind] then
			fail("context entry kind is unsupported by Amp: " .. entry.kind)
		end
		if not entry.transfer.eligible then
			fail("context entry is not transfer-eligible: " .. entry.id)
		end
		if type(entry.content) ~= "string" or entry.content == "" then
			fail("context entry lacks streamable content: " .. entry.id)
		end
		sections[index] = "## "
			.. entry.kind
			.. ": "
			.. entry.ref
			.. "\nsource: "
			.. entry.provenance.source
			.. " ("
			.. entry.provenance.ref
			.. ")\n"
			.. entry.content
	end
	opts.send({
		type = "user",
		message = { role = "user", content = { { type = "text", text = table.concat(sections, "\n\n") } } },
	})
	return #sections
end

return M
