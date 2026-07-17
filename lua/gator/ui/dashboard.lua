local M = {}
local dashboards = {}
local accessibility = require("gator.ui.accessibility")

local function fail(message)
	error("Gator dashboard: " .. message, 3)
end

local function rows(tasks)
	if type(tasks) ~= "table" or not vim.islist(tasks) then
		fail("tasks must be an array")
	end
	local result = {}
	for index, task in ipairs(tasks) do
		if type(task) ~= "table" then
			fail("task " .. index .. " must be a table")
		end
		for key in pairs(task) do
			if
				key ~= "id"
				and key ~= "objective"
				and key ~= "lifecycle"
				and key ~= "provider"
				and key ~= "workspace"
				and key ~= "review_state"
			then
				fail("task " .. index .. " contains unsupported field: " .. tostring(key))
			end
		end
		for _, key in ipairs({ "id", "objective", "lifecycle", "provider", "workspace", "review_state" }) do
			if type(task[key]) ~= "string" or task[key] == "" then
				fail("task " .. index .. " " .. key .. " must be a non-empty string")
			end
		end
		result[index] = vim.deepcopy(task)
	end
	return result
end

function M.filter(tasks, filters)
	filters = filters or {}
	if type(filters) ~= "table" then
		fail("filters must be a table")
	end
	for key in pairs(filters) do
		if key ~= "lifecycle" and key ~= "provider" and key ~= "workspace" and key ~= "review_state" then
			fail("filter is unsupported: " .. tostring(key))
		end
		if type(filters[key]) ~= "string" or filters[key] == "" then
			fail("filter values must be non-empty strings")
		end
	end
	local result = {}
	for _, task in ipairs(rows(tasks)) do
		local matches = true
		for key, value in pairs(filters) do
			if task[key] ~= value then
				matches = false
			end
		end
		if matches then
			table.insert(result, task)
		end
	end
	table.sort(result, function(left, right)
		return left.id < right.id
	end)
	return result
end

local function current()
	local tabpage = vim.api.nvim_get_current_tabpage()
	local dashboard = dashboards[tabpage]
	if dashboard and vim.api.nvim_win_is_valid(dashboard.window) then
		return dashboard, tabpage
	end
	dashboards[tabpage] = nil
	return nil, tabpage
end

local function render(dashboard)
	local lines = { "Gator tasks" }
	if #dashboard.tasks == 0 then
		table.insert(lines, "No matching tasks")
	else
		for index, task in ipairs(dashboard.tasks) do
			local marker = index == dashboard.selected and ">" or " "
			table.insert(
				lines,
				marker
					.. " "
					.. task.id
					.. " · "
					.. task.objective
					.. " · "
					.. task.lifecycle
					.. " · "
					.. task.provider
					.. " · "
					.. task.workspace
					.. " · "
					.. task.review_state
			)
		end
	end
	table.insert(lines, "j/k navigate · <CR> open · q close · ? help")
	accessibility.render(dashboard.buffer, lines, "gator-dashboard")
end

local function bind(dashboard)
	accessibility.panel(dashboard.buffer, { next = "j", previous = "k", confirm = "<CR>", cancel = "q", help = "?" }, {
		next = function()
			if #dashboard.tasks > 0 then
				M.select(dashboard.selected % #dashboard.tasks + 1)
			end
		end,
		previous = function()
			if #dashboard.tasks > 0 then
				M.select((dashboard.selected - 2) % #dashboard.tasks + 1)
			end
		end,
		confirm = function()
			if #dashboard.tasks > 0 then
				M.open_selected()
			end
		end,
		cancel = M.close,
		help = function()
			vim.notify("Gator tasks: j/k navigate, <CR> open, q close", vim.log.levels.INFO)
		end,
	})
end

function M.open(opts)
	if type(opts) ~= "table" or type(opts.on_open) ~= "function" then
		fail("open requires an on_open callback")
	end
	local tasks = M.filter(opts.tasks or {}, opts.filters)
	local dashboard, tabpage = current()
	if dashboard then
		dashboard.tasks = tasks
		dashboard.on_open = opts.on_open
		dashboard.selected = math.min(dashboard.selected, math.max(#tasks, 1))
		render(dashboard)
		vim.api.nvim_set_current_win(dashboard.window)
		return dashboard.window
	end
	vim.cmd("botright 12new")
	local window = vim.api.nvim_get_current_win()
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype = "gator-dashboard"
	vim.bo[buffer].bufhidden = "wipe"
	vim.api.nvim_win_set_buf(window, buffer)
	dashboard = { window = window, buffer = buffer, tasks = tasks, selected = 1, on_open = opts.on_open }
	dashboards[tabpage] = dashboard
	render(dashboard)
	bind(dashboard)
	return window
end

function M.select(index)
	local dashboard = current()
	if not dashboard then
		fail("no Gator dashboard is open in this tab")
	end
	if type(index) ~= "number" or index % 1 ~= 0 or index < 1 or index > #dashboard.tasks then
		fail("selection must identify a task")
	end
	dashboard.selected = index
	render(dashboard)
	return vim.deepcopy(dashboard.tasks[index])
end

function M.open_selected()
	local dashboard = current()
	if not dashboard then
		fail("no Gator dashboard is open in this tab")
	end
	local task = dashboard.tasks[dashboard.selected]
	if not task then
		fail("no task is selected")
	end
	dashboard.on_open(vim.deepcopy(task))
end

function M.close()
	local dashboard, tabpage = current()
	if not dashboard then
		return false
	end
	vim.api.nvim_win_close(dashboard.window, true)
	dashboards[tabpage] = nil
	return true
end

return M
