local runtime = require("gator.extensions.runtime")

local M = {}

local function fail(message)
	error("Gator extensions: " .. tostring(message), 3)
end

function M.open(opts)
	if type(opts) ~= "table" then
		fail("open requires options")
	end
	for key in pairs(opts) do
		if key ~= "modules" and key ~= "renderers" and key ~= "columns" then
			fail("open accepts explicit modules, renderers, and columns; directory discovery is unavailable")
		end
	end
	return runtime.new({
		modules = opts.modules or {},
		renderers = opts.renderers or {},
		columns = opts.columns or {},
	})
end

return M
