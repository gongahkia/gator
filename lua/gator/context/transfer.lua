local M = {}
local modes = { manual = true, explicit = true, automatic = true }

local function fail(message)
	error("Gator context transfer: " .. message, 3)
end

function M.summary(opts)
	if
		type(opts) ~= "table"
		or type(opts.source_provider) ~= "string"
		or opts.source_provider == ""
		or type(opts.target_provider) ~= "string"
		or opts.target_provider == ""
		or type(opts.content) ~= "string"
		or opts.content == ""
	then
		fail("summary requires source provider, target provider, and content")
	end
	if not modes[opts.mode] then
		fail("mode must be manual, explicit, or automatic")
	end
	if opts.mode == "automatic" then
		if opts.opt_in ~= true then
			return { available = false, reason = "automatic summary transfer requires opt-in" }
		end
	elseif opts.confirmed ~= true then
		return { available = false, reason = opts.mode .. " summary transfer requires confirmation" }
	end
	return {
		available = true,
		mode = opts.mode,
		source_provider = opts.source_provider,
		target_provider = opts.target_provider,
		content = opts.content,
		editable = opts.mode ~= "automatic",
	}
end

return M
