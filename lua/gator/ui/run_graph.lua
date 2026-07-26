local accessibility = require("gator.ui.accessibility")
local panel_window = require("gator.ui.window")

local M = {}
local panels = {}

local function fail(message)
	error("Gator run graph: " .. tostring(message), 3)
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
	local runs = panel.workflow:runs()
	panel.runs = runs
	panel.selected = math.min(panel.selected, math.max(#runs, 1))
	local lines = { "Gator runs", "" }
	if #runs == 0 then
		table.insert(lines, "No Gator-managed runs. Use :Gator to launch one.")
	else
		for index, run in ipairs(runs) do
			local parent = run.parent_run_id and (" ← " .. run.parent_run_id) or ""
			local usage = run.usage.state == "reported" and "reported"
				or run.usage.state == "estimated" and "estimated"
				or "unknown"
			table.insert(lines, string.format(
				"%s %s · %s · %s · %s · %s%s",
				index == panel.selected and ">" or " ",
				run.id,
				run.provider,
				run.role,
				run.state,
				usage,
				parent
			))
		end
	end
	table.insert(lines, "")
	table.insert(lines, "<CR> focus · h handoff · p parallel writer · s stop · r resume · q close · ? help")
	accessibility.render(panel.buffer, lines, "gator-runs")
end

local function selected(panel)
	return panel.runs[panel.selected]
end

local function bind(panel)
	accessibility.panel(panel.buffer, {
		next = "j",
		previous = "k",
		confirm = "<CR>",
		handoff = "h",
		parallel = "p",
		stop = "s",
		resume = "r",
		close = "q",
		help = "?",
	}, {
		next = function()
			if #panel.runs > 0 then
				panel.selected = panel.selected % #panel.runs + 1
				render(panel)
			end
		end,
		previous = function()
			if #panel.runs > 0 then
				panel.selected = (panel.selected - 2) % #panel.runs + 1
				render(panel)
			end
		end,
		confirm = function()
			local run = selected(panel)
			if run then
				panel.workflow:focus(run.id)
			end
		end,
		handoff = function()
			local run = selected(panel)
			if run then
				panel.workflow:handoff(run.id)
			end
		end,
		parallel = function()
			local run = selected(panel)
			if run then
				panel.workflow:launch_parallel(run.id)
			end
		end,
		stop = function()
			local run = selected(panel)
			if run then
				panel.workflow:stop(run.id)
				render(panel)
			end
		end,
		resume = function()
			local run = selected(panel)
			if run then
				panel.workflow:resume(run.id)
			end
		end,
		close = M.close,
		help = function()
			vim.notify("Gator runs: <CR> focus, h handoff, p parallel writer, s stop, r resume, q close", vim.log.levels.INFO)
		end,
	})
end

function M.open(workflow)
	if type(workflow) ~= "table" or type(workflow.runs) ~= "function" then
		fail("open requires a run workflow")
	end
	local panel, tabpage = current()
	if panel then
		panel.workflow = workflow
		render(panel)
		vim.api.nvim_set_current_win(panel.window)
		return panel.window
	end
	local opened = panel_window.open("botright 18new")
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype, vim.bo[buffer].bufhidden = "gator-runs", "wipe"
	vim.api.nvim_win_set_buf(opened.window, buffer)
	panel = { window = opened.window, buffer = buffer, previous = opened.previous, workflow = workflow, runs = {}, selected = 1 }
	panels[tabpage] = panel
	render(panel)
	bind(panel)
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
