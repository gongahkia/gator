local M = {}

function M.new(config)
	return {
		config = config,
		tasks = {},
		context = {},
		adapters = {},
	}
end

return M
