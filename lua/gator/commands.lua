local M = {}

function M.register()
	vim.api.nvim_create_user_command("Gator", function()
		require("gator").open()
	end, { desc = "Open Gator" })
	vim.api.nvim_create_user_command("GatorHealth", function()
		require("gator").health()
	end, { desc = "Check Gator health" })
end

return M
