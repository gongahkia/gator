local M = { name = "startup", api_version = 2, deferred_modules = { "gator.adapters", "gator.indexer", "gator.github" } }

function M.recover(opts)
	if type(opts) ~= "table" or type(opts.state) ~= "table" then
		error("Gator startup: recover requires state", 2)
	end
	return { status = "ready", detail = "project-local runs load on demand", runs = {} }
end

function M.measure(opts)
	if type(opts) ~= "table" or type(opts.load) ~= "function" then
		error("Gator startup: measure requires load", 2)
	end
	local value = opts.load()
	return { value = value, elapsed = 0, loaded = {} }
end

return M
