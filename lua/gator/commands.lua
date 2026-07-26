local M = {}

function M.register()
	vim.api.nvim_create_user_command("Gator", function()
		require("gator").dispatch("open")
	end, { desc = "Open Gator" })
	vim.api.nvim_create_user_command("GatorHealth", function()
		require("gator").dispatch("health")
	end, { desc = "Check Gator health" })
	vim.api.nvim_create_user_command("GatorExportDiagnostics", function()
		vim.schedule(function()
			local ok, result = pcall(require("gator").export_diagnostics)
			local message = ok and (result.state == "ready" and "Diagnostic export: " .. result.path or result.reason)
				or require("gator.policy.redact").text(tostring(result))
			vim.notify(
				message,
				ok and result.state == "ready" and vim.log.levels.INFO or vim.log.levels.WARN,
				{ title = "Gator" }
			)
		end)
	end, { desc = "Write a local redacted diagnostic export" })
	vim.api.nvim_create_user_command("GatorBetaReadiness", function()
		vim.schedule(function()
			local ok, result = pcall(require("gator").verify_beta_readiness)
			local message = ok and ("Public-beta readiness: " .. result.state .. " · " .. result.path)
				or require("gator.policy.redact").text(tostring(result))
			vim.notify(
				message,
				ok and result.state == "ready" and vim.log.levels.INFO or vim.log.levels.WARN,
				{ title = "Gator" }
			)
		end)
	end, { desc = "Verify public-beta readiness and write a local failure report" })
	vim.api.nvim_create_user_command("GatorStopSession", function()
		local ok, result = pcall(require("gator").stop_session)
		vim.notify(
			ok and "Gator session stopped; provider session remains resumable" or require("gator.policy.redact").text(tostring(result)),
			ok and vim.log.levels.INFO or vim.log.levels.ERROR,
			{ title = "Gator" }
		)
	end, { desc = "Stop the active managed provider process" })
	vim.api.nvim_create_user_command("GatorCaptureSelection", function(opts)
		require("gator").dispatch("capture_selection", {
			target = opts.args,
			buffer = vim.api.nvim_get_current_buf(),
			first_line = opts.line1,
			last_line = opts.line2,
		})
	end, {
		nargs = 1,
		range = true,
		desc = "Capture visual selection for task:<id> or session:<provider>:<id>",
	})
	vim.api.nvim_create_user_command("GatorPalette", function(opts)
		require("gator").dispatch("palette", { id = opts.args })
	end, {
		nargs = "?",
		complete = function(arglead)
			local gator = require("gator")
			if not gator._coordinator then
				gator.setup()
			end
			pcall(gator._coordinator.workflow, gator._coordinator)
			return require("gator.ui").palette.complete(arglead)
		end,
		desc = "Open or run a registered Gator command palette entry",
	})
end

return M
