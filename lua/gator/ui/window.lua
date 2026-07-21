local M = {}

function M.open(command)
	if type(command) ~= "string" or command == "" then
		error("Gator panel window: open requires a command", 2)
	end
	local previous = vim.api.nvim_get_current_win()
	vim.cmd(command)
	return { window = vim.api.nvim_get_current_win(), previous = previous }
end

function M.close(window, previous)
	if vim.api.nvim_win_is_valid(window) then
		vim.api.nvim_win_close(window, true)
	end
	if previous and vim.api.nvim_win_is_valid(previous) then
		vim.api.nvim_set_current_win(previous)
	end
end

return M
