local capabilities = require("gator.adapters.capabilities")
local M = {}

local function fail(message)
	error("Gator context estimate: " .. message, 3)
end

function M.tokens(opts)
	if type(opts) ~= "table" or not capabilities.is(opts.capabilities) or type(opts.text) ~= "string" then
		fail("tokens requires a capability contract and text")
	end
	if opts.count ~= nil and type(opts.count) ~= "function" then
		fail("count must be a function")
	end
	local available, reason = capabilities.supports(opts.capabilities, "model", "token_count")
	if not available then
		return { status = "unavailable", reason = reason }
	end
	if not opts.count then
		return { status = "unavailable", reason = "provider token counter is unavailable" }
	end
	local ok, count = pcall(opts.count, opts.text)
	if not ok or type(count) ~= "number" or count < 0 or count % 1 ~= 0 then
		return { status = "unavailable", reason = "provider token counter returned invalid data" }
	end
	return { status = "estimated", tokens = math.ceil(count * 1.1) }
end

return M
