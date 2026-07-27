local M = {}

function M.register()
	vim.api.nvim_create_user_command("Gator", function(opts)
		require("gator").dispatch("open", {
			provider = opts.args ~= "" and opts.args or nil,
			buffer = vim.api.nvim_get_current_buf(),
			first_line = opts.range > 0 and opts.line1 or nil,
			last_line = opts.range > 0 and opts.line2 or nil,
		})
	end, { nargs = "?", range = true, desc = "Launch a coding agent with current context" })
	vim.api.nvim_create_user_command("GatorRuns", function()
		require("gator").dispatch("runs")
	end, { desc = "Open Gator run graph" })
	vim.api.nvim_create_user_command("GatorHandoff", function(opts)
		local run_id = opts.fargs[1]
		if not run_id then
			error("GatorHandoff requires a source run id; use :GatorRuns to inspect runs", 0)
		end
		require("gator").dispatch("handoff", { run_id = run_id, provider = opts.fargs[2] })
	end, { nargs = "+", desc = "Review and launch a provider handoff" })
	vim.api.nvim_create_user_command("GatorSend", function(opts)
		local args = opts.fargs
		require("gator").dispatch("send_context", {
			run_id = args[1],
			kind = args[2],
			bundle_id = args[3],
			buffer = vim.api.nvim_get_current_buf(),
			first_line = opts.range > 0 and opts.line1 or nil,
			last_line = opts.range > 0 and opts.line2 or nil,
		})
	end, { nargs = "*", range = true, desc = "Send selected editor context to an active Gator chat" })
	vim.api.nvim_create_user_command("GatorReview", function(opts)
		require("gator").dispatch("review", { run_id = opts.args ~= "" and opts.args or nil })
	end, { nargs = "?", desc = "Review a Gator run diff and approved test evidence" })
	vim.api.nvim_create_user_command("GatorRunbook", function()
		require("gator").dispatch("runbook_next")
	end, { nargs = 0, desc = "Select and start one ready Gator runbook step" })
	vim.api.nvim_create_user_command("GatorPrune", function()
		require("gator").dispatch("prune")
	end, { nargs = 0, desc = "Preview and remove expired Gator-owned artifacts" })
	vim.api.nvim_create_user_command("GatorForget", function(opts)
		local ok, result = pcall(require("gator").dispatch, "forget", { run_id = opts.args })
		vim.notify(
			ok and (result and "Gator run forgotten" or "Gator run is unavailable") or tostring(result),
			ok and vim.log.levels.INFO or vim.log.levels.ERROR,
			{ title = "Gator" }
		)
	end, { nargs = 1, desc = "Forget a completed Gator run and eligible worktree" })
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
	vim.api.nvim_create_user_command("GatorStopSession", function(opts)
		local ok, result =
			pcall(require("gator").dispatch, "stop_session", { run_id = opts.args ~= "" and opts.args or nil })
		vim.notify(
			ok and "Gator session stopped; provider session remains resumable"
				or require("gator.policy.redact").text(tostring(result)),
			ok and vim.log.levels.INFO or vim.log.levels.ERROR,
			{ title = "Gator" }
		)
	end, { nargs = "?", desc = "Stop an active Gator-managed run" })
end

return M
