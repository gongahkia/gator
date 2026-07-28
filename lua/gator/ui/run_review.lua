local accessibility = require("gator.ui.accessibility")
local panel_window = require("gator.ui.window")
local redact = require("gator.policy.redact")

local M = {}
local panels = {}

local function fail(message)
	error("Gator run review: " .. redact.text(tostring(message)), 3)
end

local function current()
	local tabpage = vim.api.nvim_get_current_tabpage()
	local panel = panels[tabpage]
	if panel and vim.api.nvim_win_is_valid(panel.window) then
		return panel, tabpage
	end
	panels[tabpage] = nil
	return nil, tabpage
end

local function render(panel)
	local lines = {
		"Gator review",
		"Run: " .. panel.run.id .. " · provider: " .. panel.run.provider .. " · workspace: " .. panel.workspace,
		"Base SHA: " .. panel.base_sha,
		"Reviewed diff SHA256: " .. panel.diff_sha256,
		"",
		"## Worktree diff",
		"",
		"```diff",
		panel.diff ~= "" and panel.diff or "No diff relative to HEAD.",
		"```",
		"",
		"Approved tests: " .. (#panel.commands > 0 and table.concat(panel.commands, ", ") or "none configured"),
		"<CR> review action · q close · ? help",
	}
	accessibility.render(panel.buffer, lines, "gator-review")
end

local function action(panel)
	local choices = { "Accept review", "Request changes", "Create handoff", "Cancel" }
	if #panel.commands > 0 then
		table.insert(choices, 1, "Run approved test")
	end
	vim.ui.select(choices, { prompt = "Gator review action" }, function(choice)
		if choice == "Run approved test" then
			vim.ui.select(panel.commands, { prompt = "Run approved Gator test" }, function(command)
				if command then
					panel.on_test(command)
				end
			end)
		elseif choice == "Accept review" then
			panel.on_decision("accepted")
		elseif choice == "Request changes" then
			panel.on_decision("changes_requested")
		elseif choice == "Create handoff" then
			panel.on_decision("handoff")
		end
	end)
end

function M.open(opts)
	if
		type(opts) ~= "table"
		or type(opts.run) ~= "table"
		or type(opts.workspace) ~= "string"
		or type(opts.base_sha) ~= "string"
		or type(opts.diff_sha256) ~= "string"
		or type(opts.diff) ~= "string"
		or type(opts.commands) ~= "table"
		or not vim.islist(opts.commands)
		or type(opts.on_test) ~= "function"
		or type(opts.on_decision) ~= "function"
	then
		fail("open requires run, workspace, base SHA, diff, commands, and callbacks")
	end
	for _, command in ipairs(opts.commands) do
		if type(command) ~= "string" or command == "" then
			fail("commands must contain non-empty identifiers")
		end
	end
	local panel, tabpage = current()
	if panel then
		panel.run = vim.deepcopy(opts.run)
		panel.workspace = opts.workspace
		panel.base_sha = opts.base_sha
		panel.diff_sha256 = opts.diff_sha256
		panel.diff = redact.text(opts.diff)
		panel.commands = vim.deepcopy(opts.commands)
		panel.on_test = opts.on_test
		panel.on_decision = opts.on_decision
		render(panel)
		vim.api.nvim_set_current_win(panel.window)
		return panel.window
	end
	local opened = panel_window.open("botright 20new")
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype, vim.bo[buffer].bufhidden = "gator-review", "wipe"
	vim.api.nvim_win_set_buf(opened.window, buffer)
	panel = {
		window = opened.window,
		buffer = buffer,
		previous = opened.previous,
		run = vim.deepcopy(opts.run),
		workspace = opts.workspace,
		base_sha = opts.base_sha,
		diff_sha256 = opts.diff_sha256,
		diff = redact.text(opts.diff),
		commands = vim.deepcopy(opts.commands),
		on_test = opts.on_test,
		on_decision = opts.on_decision,
	}
	panels[tabpage] = panel
	render(panel)
	accessibility.panel(buffer, { confirm = "<CR>", cancel = "q", help = "?" }, {
		confirm = function()
			action(panel)
		end,
		cancel = M.close,
		help = function()
			require("gator.ui.notice").show(
				"Gator review: <CR> choose test, accept, request changes, or handoff; q closes",
				vim.log.levels.INFO
			)
		end,
	})
	return panel.window
end

function M.close()
	local panel, tabpage = current()
	if not panel then
		return false
	end
	panel_window.close(panel.window, panel.previous)
	panels[tabpage] = nil
	return true
end

return M
