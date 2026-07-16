local M = {}

M.defaults = {
	ui = { layout = "adaptive", keymaps = {}, screen_reader = true },
	context = { mode = "manual", trust = "provenance" },
	sessions = { transfer = "manual" },
	workspaces = { mode = "project", max_write_runs = 1 },
	persistence = { sharing = "local" },
	telemetry = { enabled = false },
}

function M.resolve(opts)
	opts = opts or {}
	local config = vim.tbl_deep_extend("force", M.defaults, opts)
	if not vim.tbl_contains({ "adaptive", "modal" }, config.ui.layout) then
		error("gator.ui.layout must be adaptive or modal")
	end
	if type(config.ui.keymaps) ~= "table" then
		error("gator.ui.keymaps must be a table")
	end
	if type(config.ui.screen_reader) ~= "boolean" then
		error("gator.ui.screen_reader must be boolean")
	end
	if not vim.tbl_contains({ "manual", "inspect", "automatic" }, config.context.mode) then
		error("gator.context.mode must be manual, inspect, or automatic")
	end
	if config.workspaces.max_write_runs < 1 then
		error("gator.workspaces.max_write_runs must be positive")
	end
	return config
end

return M
