local pack = require("gator.context.pack")
local inspector = require("gator.ui.context_inspector")
local M = {}

local function fail(message)
	error("Gator inspect context: " .. message, 3)
end

function M.suggest(opts)
	if type(opts) ~= "table" or not pack.is(opts.pack) or type(opts.on_confirm) ~= "function" then
		fail("suggest requires a context pack and confirmation callback")
	end
	return inspector.open({ pack = opts.pack, on_confirm = opts.on_confirm })
end

return M
