local M = { name = "ui", api_version = 1 }

function M.open(state)
	local buf = vim.api.nvim_create_buf(false, true)
	vim.bo[buf].filetype = "gator"
	vim.bo[buf].bufhidden = "wipe"
	vim.api.nvim_buf_set_lines(buf, 0, -1, false, {
		"Gator",
		"Foundation workspace is active.",
		"Adapter, context, task, and review panels are issue-tracked.",
		"Configured context mode: " .. state.config.context.mode,
	})
	vim.cmd("botright split")
	vim.api.nvim_win_set_buf(0, buf)
end

return M
