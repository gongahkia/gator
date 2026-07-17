local M = {}

function M.new(config)
	return {
		config = config,
		tasks = {},
		context = {},
		adapters = {},
		workspace = { status = "ready" },
		review = {},
	}
end

return M
