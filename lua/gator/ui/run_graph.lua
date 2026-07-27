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

local default_resources = { enabled = true, fields = { "wall_time", "context_bytes", "worktree", "usage" } }

local function resource_config(workflow)
	if type(workflow.resource_display) ~= "function" then
		return default_resources
	end
	local ok, value = pcall(workflow.resource_display, workflow)
	if not ok or type(value) ~= "table" or type(value.enabled) ~= "boolean" or type(value.fields) ~= "table" then
		return default_resources
	end
	return value
end

local function selected_fields(value)
	local result = {}
	for _, field in ipairs(value.fields) do
		result[field] = true
	end
	return result
end

local function bytes(value)
	if value < 1024 then
		return value .. " B"
	end
	if value < 1024 * 1024 then
		return string.format("%.1f KiB", value / 1024)
	end
	if value < 1024 * 1024 * 1024 then
		return string.format("%.1f MiB", value / (1024 * 1024))
	end
	return string.format("%.1f GiB", value / (1024 * 1024 * 1024))
end

local function duration(value)
	if value < 60 then
		return value .. "s"
	end
	if value < 60 * 60 then
		return math.floor(value / 60) .. "m " .. value % 60 .. "s"
	end
	return math.floor(value / 3600) .. "h " .. math.floor(value % 3600 / 60) .. "m"
end

local function render(panel)
	local runs = panel.workflow:runs()
	panel.runs = runs
	panel.selected = math.min(panel.selected, math.max(#runs, 1))
	local lines = { "Gator runs", "" }
	local configured_resources = resource_config(panel.workflow)
	local resource_fields = selected_fields(configured_resources)
	if
		configured_resources.enabled
		and resource_fields.worktree
		and type(panel.workflow.resource_summary) == "function"
	then
		local ok, summary = pcall(panel.workflow.resource_summary, panel.workflow)
		if ok and type(summary) == "table" and type(summary.count) == "number" then
			local size = summary.state == "measured" and bytes(summary.bytes)
				or (bytes(summary.bytes or 0) .. " + unknown")
			table.insert(lines, "Local worktrees: " .. summary.count .. " · " .. size)
			table.insert(lines, "")
		end
	end
	if #runs == 0 then
		table.insert(lines, "No Gator-managed runs. Use :Gator to launch one.")
	else
		for index, run in ipairs(runs) do
			local parent = run.parent_run_id and (" ← " .. run.parent_run_id) or ""
			local usage = run.usage.state == "reported"
					and string.format(
						"reported · in %s · out %s · total %s",
						run.usage.input_tokens or "?",
						run.usage.output_tokens or "?",
						run.usage.total_tokens or "?"
					)
				or (run.usage.state == "estimated" and ("estimated · context ~" .. (run.usage.input_tokens or "?") .. " tokens"))
				or ("unknown · context estimate ~" .. (run.usage.context_tokens_estimate or "?") .. " tokens")
			local budget = run.budget.limit_tokens == 0 and "unbounded"
				or (run.budget.state .. " · " .. (run.usage.total_tokens or "?") .. "/" .. run.budget.limit_tokens)
			local workspace = vim.fn.fnamemodify(run.workspace.root, ":~:.")
			table.insert(
				lines,
				string.format(
					"%s %s · %s · %s · %s · %s%s",
					index == panel.selected and ">" or " ",
					run.id,
					run.provider,
					run.role,
					run.state,
					usage,
					parent
				)
			)
			table.insert(lines, "  Workspace: " .. run.workspace.kind .. " · " .. workspace)
			table.insert(
				lines,
				"  Context: " .. (run.bundle_id or "unavailable") .. " · transcript " .. run.transcript
			)
			if configured_resources.enabled then
				local measurements
				if type(panel.workflow.run_resources) == "function" then
					local ok, value = pcall(panel.workflow.run_resources, panel.workflow, run)
					measurements = ok and value or nil
				end
				local values = {}
				if resource_fields.wall_time then
					table.insert(values, "wall " .. (measurements and duration(measurements.wall_seconds) or "unknown"))
				end
				if resource_fields.context_bytes then
					if measurements then
						table.insert(
							values,
							"context "
								.. bytes(measurements.context_bytes)
								.. " / "
								.. measurements.context_sends
								.. " sends"
						)
					else
						table.insert(values, "context unknown")
					end
				end
				if resource_fields.worktree and run.workspace.kind == "worktree" then
					local worktree = measurements and measurements.worktree
					table.insert(
						values,
						"worktree "
							.. (worktree and worktree.state == "measured" and bytes(worktree.bytes) or "unknown")
					)
				end
				if resource_fields.usage then
					table.insert(values, "provider usage " .. usage)
				end
				if #values > 0 then
					table.insert(lines, "  Resources: " .. table.concat(values, " · "))
				end
			end
			table.insert(lines, "  Budget: " .. budget)
			local trust = run.trust
			if trust then
				local write = trust.write.state .. (trust.write.mode and (" (" .. trust.write.mode .. ")") or "")
				local policy = trust.policy.state .. (trust.policy.mode and (" (" .. trust.policy.mode .. ")") or "")
				table.insert(
					lines,
					"  Trust: "
						.. trust.surface
						.. " · "
						.. trust.security_owner
						.. " · policy "
						.. policy
						.. " · write "
						.. write
						.. " · network "
						.. trust.network.state
						.. " · MCP "
						.. trust.mcp.state
						.. " · approval "
						.. trust.approval.state
				)
			else
				table.insert(lines, "  Trust: legacy run · launch trust unavailable")
			end
			if type(panel.workflow.events) == "function" then
				local ok, events = pcall(panel.workflow.events, panel.workflow, run.id)
				table.insert(
					lines,
					ok and ("  Journal: " .. #events .. " Gator-owned events · l details") or "  Journal: unavailable"
				)
			end
			if run.workspace.kind == "worktree" and type(panel.workflow.worktree_lease) == "function" then
				local ok, lease = pcall(panel.workflow.worktree_lease, panel.workflow, run.id)
				if ok and lease then
					table.insert(lines, "  Worktree lease: " .. lease.state .. " · " .. lease.branch)
				else
					table.insert(lines, "  Worktree lease: unavailable")
				end
			end
		end
	end
	if type(panel.workflow.runbooks) == "function" and type(panel.workflow.runbook_status) == "function" then
		local ok, runbooks = pcall(panel.workflow.runbooks, panel.workflow)
		if ok and #runbooks > 0 then
			table.insert(lines, "")
			table.insert(lines, "Gator runbooks")
			for _, runbook in ipairs(runbooks) do
				local available, status = pcall(panel.workflow.runbook_status, panel.workflow, runbook.id)
				if available then
					local limit = status.max_tokens == 0 and "unbounded" or tostring(status.max_tokens)
					table.insert(
						lines,
						"- "
							.. status.id
							.. " · active "
							.. status.active
							.. " · reported "
							.. status.reported_tokens
							.. "/"
							.. limit
							.. " · usage "
							.. status.usage_state
					)
					for _, step in ipairs(status.steps) do
						local dependencies = #step.depends_on == 0 and "none" or table.concat(step.depends_on, ",")
						table.insert(
							lines,
							"  "
								.. (step.ready and ">" or "·")
								.. " "
								.. step.id
								.. " · "
								.. step.role
								.. " · "
								.. step.state
								.. " · depends "
								.. dependencies
						)
					end
				end
			end
		end
	end
	table.insert(lines, "")
	table.insert(
		lines,
		"<CR> focus · c context · f native fork · h handoff · l journal · p parallel writer · v review · n next runbook step · s stop · r resume · q close · ? help"
	)
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
		journal = "l",
		fork = "f",
		context = "c",
		review = "v",
		parallel = "p",
		runbook = "n",
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
		journal = function()
			local run = selected(panel)
			if run and type(panel.workflow.open_events) == "function" then
				panel.workflow:open_events(run.id)
			end
		end,
		fork = function()
			local run = selected(panel)
			if run then
				panel.workflow:fork(run.id)
			end
		end,
		context = function()
			local run = selected(panel)
			if run then
				panel.workflow:attach_context(run.id)
			end
		end,
		review = function()
			local run = selected(panel)
			if run then
				panel.workflow:review(run.id)
			end
		end,
		parallel = function()
			local run = selected(panel)
			if run then
				panel.workflow:launch_parallel(run.id)
			end
		end,
		runbook = function()
			if type(panel.workflow.start_ready_runbook_step) == "function" then
				panel.workflow:start_ready_runbook_step()
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
			vim.notify(
				"Gator runs: <CR> focus, c context, f native fork, h handoff, l journal, p parallel writer, v review, n next runbook step, s stop, r resume, q close",
				vim.log.levels.INFO
			)
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
	panel = {
		window = opened.window,
		buffer = buffer,
		previous = opened.previous,
		workflow = workflow,
		runs = {},
		selected = 1,
	}
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
