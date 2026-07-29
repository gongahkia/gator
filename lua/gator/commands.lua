local M = {}
local registered = false
local installed_ask_mapping = nil
local installed_edit_mapping = nil
local notice = require("gator.ui.notice")

local function mapping(lhs)
	local value = vim.fn.maparg(lhs, "x", false, true)
	return type(value) == "table" and next(value) and value or nil
end

local function remove_default_mapping(kind)
	local installed = kind == "edit" and installed_edit_mapping or installed_ask_mapping
	if not installed then
		return
	end
	local rhs = kind == "edit" and "<Plug>(gator-edit-selection)" or "<Plug>(gator-ask-selection)"
	local current = mapping(installed)
	if current and current.rhs == rhs then
		vim.keymap.del("x", installed)
	end
	if kind == "edit" then
		installed_edit_mapping = nil
	else
		installed_ask_mapping = nil
	end
end

function M.configure(opts)
	if type(opts) ~= "table" or type(opts.ask_selection) ~= "table" or type(opts.edit_selection) ~= "table" then
		error("Gator commands: settings require ask_selection and edit_selection", 3)
	end
	for _, value in ipairs({ opts.ask_selection.keymap, opts.edit_selection.keymap }) do
		if value ~= false and (type(value) ~= "string" or value == "") then
			error("Gator commands: keymap must be false or non-empty text", 3)
		end
	end
	vim.keymap.set("x", "<Plug>(gator-ask-selection)", ":<C-U>'<,'>GatorAsk<CR>", {
		desc = "Gator ask about selected text",
		silent = true,
	})
	vim.keymap.set("x", "<Plug>(gator-edit-selection)", ":<C-U>'<,'>GatorEdit<CR>", {
		desc = "Gator edit selected text",
		silent = true,
	})
	remove_default_mapping("ask")
	remove_default_mapping("edit")
	local installed = { ask = false, edit = false }
	if opts.ask_selection.keymap ~= false and not mapping(opts.ask_selection.keymap) then
		vim.keymap.set("x", opts.ask_selection.keymap, "<Plug>(gator-ask-selection)", {
			desc = "Gator ask about selected text",
			silent = true,
		})
		installed_ask_mapping, installed.ask = opts.ask_selection.keymap, true
	end
	if opts.edit_selection.keymap ~= false and not mapping(opts.edit_selection.keymap) then
		vim.keymap.set("x", opts.edit_selection.keymap, "<Plug>(gator-edit-selection)", {
			desc = "Gator edit selected text",
			silent = true,
		})
		installed_edit_mapping, installed.edit = opts.edit_selection.keymap, true
	end
	return installed
end

function M.register()
	if registered then
		return false
	end
	registered = true
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
	vim.api.nvim_create_user_command("GatorWorkspace", function(opts)
		require("gator").workspace(opts.args)
	end, { nargs = 1, desc = "Attach a Gator run workspace to the current tab" })
	vim.api.nvim_create_user_command("GatorEvents", function(opts)
		require("gator").dispatch("events", { run_id = opts.args })
	end, { nargs = 1, desc = "Open the append-only Gator event journal for a run" })
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
	vim.api.nvim_create_user_command("GatorAsk", function(opts)
		if opts.range == 0 then
			error("GatorAsk requires a Visual line selection", 0)
		end
		require("gator").dispatch("ask_selection", {
			run_id = opts.args ~= "" and opts.args or nil,
			buffer = vim.api.nvim_get_current_buf(),
			first_line = opts.line1,
			last_line = opts.line2,
		})
	end, { nargs = "?", range = true, desc = "Ask an active Gator chat about selected lines" })
	vim.api.nvim_create_user_command("GatorEdit", function(opts)
		if opts.range == 0 then
			error("GatorEdit requires a Visual line selection", 0)
		end
		require("gator").dispatch("edit", {
			run_id = opts.args ~= "" and opts.args or nil,
			buffer = vim.api.nvim_get_current_buf(),
			first_line = opts.line1,
			last_line = opts.line2,
		})
	end, { nargs = "?", range = true, desc = "Request a reviewed replacement for selected text" })
	vim.api.nvim_create_user_command("GatorReview", function(opts)
		require("gator").dispatch("review", { run_id = opts.args ~= "" and opts.args or nil })
	end, { nargs = "?", desc = "Review a Gator run diff and approved test evidence" })
	vim.api.nvim_create_user_command("GatorRunbook", function()
		require("gator").dispatch("runbook_next")
	end, { nargs = 0, desc = "Select and start one ready Gator runbook step" })
	vim.api.nvim_create_user_command("GatorPrune", function()
		require("gator").dispatch("prune")
	end, { nargs = 0, desc = "Preview and remove aged or quota-selected Gator artifacts" })
	vim.api.nvim_create_user_command("GatorStorage", function()
		require("gator").dispatch("storage")
	end, { nargs = 0, desc = "Inspect Gator-owned local artifact storage" })
	vim.api.nvim_create_user_command("GatorForget", function(opts)
		local ok, result = pcall(require("gator").dispatch, "forget", { run_id = opts.args })
		notice.show(
			ok and (result and "Gator run forgotten" or "Gator run is unavailable") or tostring(result),
			ok and vim.log.levels.INFO or vim.log.levels.ERROR,
			{ title = "Gator" }
		)
	end, { nargs = 1, desc = "Forget a completed Gator run and eligible worktree" })
	vim.api.nvim_create_user_command("GatorHealth", function(opts)
		require("gator").dispatch("health", { verbose = opts.bang })
	end, { bang = true, desc = "Check Gator health (! shows individual provider diagnostics)" })
	vim.api.nvim_create_user_command("GatorCompletion", function(opts)
		local ok, value = pcall(require("gator").completion_command, opts.args)
		local status = ok and value or nil
		local message = ok
				and ("Completion " .. (status.enabled and (status.configured and "ready" or "needs sidecar.argv") or "disabled"))
			or require("gator.policy.redact").text(tostring(value))
		notice.show(message, ok and vim.log.levels.INFO or vim.log.levels.ERROR, { title = "Gator" })
	end, {
		nargs = "?",
		complete = function()
			return { "enable", "disable", "toggle", "status", "restart" }
		end,
		desc = "Control Gator inline completion",
	})
	vim.api.nvim_create_user_command("GatorExportDiagnostics", function()
		vim.schedule(function()
			local ok, result = pcall(require("gator").export_diagnostics)
			local message = ok
					and (result.state == "ready" and "Diagnostics saved locally" or "Diagnostics " .. result.state .. ": " .. result.reason)
				or require("gator.policy.redact").text(tostring(result))
			notice.show(
				message,
				ok and result.state == "ready" and vim.log.levels.INFO or vim.log.levels.WARN,
				{ title = "Gator" }
			)
		end)
	end, { desc = "Write a local redacted diagnostic export" })
	vim.api.nvim_create_user_command("GatorBetaReadiness", function()
		vim.schedule(function()
			local ok, result = pcall(require("gator").verify_beta_readiness)
			local message = ok
					and (result.state == "ready" and "Readiness check passed; report saved locally" or "Readiness check " .. result.state .. ": " .. result.reason)
				or require("gator.policy.redact").text(tostring(result))
			notice.show(
				message,
				ok and result.state == "ready" and vim.log.levels.INFO or vim.log.levels.WARN,
				{ title = "Gator" }
			)
		end)
	end, { desc = "Verify public-beta readiness and write a local failure report" })
	vim.api.nvim_create_user_command("GatorStopSession", function(opts)
		local ok, result =
			pcall(require("gator").dispatch, "stop_session", { run_id = opts.args ~= "" and opts.args or nil })
		notice.show(
			ok and "Gator session stopped; provider session remains resumable"
				or require("gator.policy.redact").text(tostring(result)),
			ok and vim.log.levels.INFO or vim.log.levels.ERROR,
			{ title = "Gator" }
		)
	end, { nargs = "?", desc = "Stop an active Gator-managed run" })
	return true
end

return M
