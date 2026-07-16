local M = {}

function M.register()
	vim.api.nvim_create_user_command("Gator", function()
		require("gator").open()
	end, { desc = "Open Gator" })
	vim.api.nvim_create_user_command("GatorHealth", function()
		require("gator").health()
	end, { desc = "Check Gator health" })
	vim.api.nvim_create_user_command("GatorCaptureSelection", function(opts)
		local gator = require("gator")
		if not gator._state then
			error("Gator must be set up before capturing context", 0)
		end
		require("gator.ui").selection.capture(gator._state, opts.args, {
			buffer = vim.api.nvim_get_current_buf(),
			first_line = opts.line1,
			last_line = opts.line2,
		})
	end, {
		nargs = 1,
		range = true,
		desc = "Capture visual selection for task:<id> or session:<provider>:<id>",
	})
end

return M
