local M = {}
local dashboards = {}
local kinds = { project = true, worktree = true }
local collision_kinds = { generated = true, overlap = true }
local accessibility = require("gator.ui.accessibility")
local panel_window = require("gator.ui.window")

local function fail(message)
	error("Gator workspace dashboard: " .. message, 3)
end

local function require_string(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty string")
	end
	return value
end

local function strings(value, name)
	if type(value) ~= "table" or not vim.islist(value) then
		fail(name .. " must be an array")
	end
	local result = {}
	for index, item in ipairs(value) do
		result[index] = require_string(item, name .. " " .. index)
	end
	return result
end

local function activity(value, index)
	if type(value) ~= "table" then
		fail("workspace " .. index .. " activity must be a table")
	end
	for key in pairs(value) do
		if key ~= "provider" and key ~= "session_id" and key ~= "state" then
			fail("workspace " .. index .. " activity contains unsupported field: " .. tostring(key))
		end
	end
	return {
		provider = require_string(value.provider, "workspace " .. index .. " activity provider"),
		session_id = require_string(value.session_id, "workspace " .. index .. " activity session_id"),
		state = require_string(value.state, "workspace " .. index .. " activity state"),
	}
end

local function collisions(value, index)
	if value == nil then
		return {}
	end
	if type(value) ~= "table" or not vim.islist(value) then
		fail("workspace " .. index .. " collisions must be an array")
	end
	local result = {}
	for collision_index, collision in ipairs(value) do
		if type(collision) ~= "table" then
			fail("workspace " .. index .. " collision " .. collision_index .. " must be a table")
		end
		for key in pairs(collision) do
			if key ~= "kind" and key ~= "path" and key ~= "worktree_ids" then
				fail(
					"workspace "
						.. index
						.. " collision "
						.. collision_index
						.. " contains unsupported field: "
						.. tostring(key)
				)
			end
		end
		local kind =
			require_string(collision.kind, "workspace " .. index .. " collision " .. collision_index .. " kind")
		if not collision_kinds[kind] then
			fail("workspace " .. index .. " collision " .. collision_index .. " kind is unknown: " .. kind)
		end
		result[collision_index] = {
			kind = kind,
			path = require_string(collision.path, "workspace " .. index .. " collision " .. collision_index .. " path"),
			worktree_ids = strings(
				collision.worktree_ids,
				"workspace " .. index .. " collision " .. collision_index .. " worktree_ids"
			),
		}
		if #result[collision_index].worktree_ids == 0 then
			fail("workspace " .. index .. " collision " .. collision_index .. " must identify writer worktrees")
		end
	end
	return result
end

local function workspaces(value)
	if type(value) ~= "table" or not vim.islist(value) then
		fail("workspaces must be an array")
	end
	local result = {}
	for index, workspace in ipairs(value) do
		if type(workspace) ~= "table" then
			fail("workspace " .. index .. " must be a table")
		end
		for key in pairs(workspace) do
			if
				key ~= "id"
				and key ~= "kind"
				and key ~= "root"
				and key ~= "tasks"
				and key ~= "dirty_files"
				and key ~= "activity"
				and key ~= "collisions"
			then
				fail("workspace " .. index .. " contains unsupported field: " .. tostring(key))
			end
		end
		local kind = require_string(workspace.kind, "workspace " .. index .. " kind")
		if not kinds[kind] then
			fail("workspace " .. index .. " kind must be project or worktree")
		end
		local activity_values = workspace.activity == nil and {} or workspace.activity
		local records = {}
		if type(activity_values) ~= "table" or not vim.islist(activity_values) then
			fail("workspace " .. index .. " activity must be an array")
		end
		for activity_index, value in ipairs(activity_values) do
			records[activity_index] = activity(value, index)
		end
		result[index] = {
			id = require_string(workspace.id, "workspace " .. index .. " id"),
			kind = kind,
			root = require_string(workspace.root, "workspace " .. index .. " root"),
			tasks = strings((workspace.tasks == nil and {} or workspace.tasks), "workspace " .. index .. " tasks"),
			dirty_files = strings(
				(workspace.dirty_files == nil and {} or workspace.dirty_files),
				"workspace " .. index .. " dirty_files"
			),
			activity = records,
			collisions = collisions(workspace.collisions, index),
		}
	end
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
	local lines = { "Gator workspaces" }
	if #dashboard.workspaces == 0 then
		table.insert(lines, "No inspected workspaces")
	end
	for index, workspace in ipairs(dashboard.workspaces) do
		local marker = index == dashboard.selected and ">" or " "
		table.insert(lines, marker .. " " .. workspace.id .. " · " .. workspace.kind .. " · " .. workspace.root)
		table.insert(lines, "  tasks: " .. (#workspace.tasks == 0 and "none" or table.concat(workspace.tasks, ", ")))
		table.insert(
			lines,
			"  dirty: " .. (#workspace.dirty_files == 0 and "clean" or table.concat(workspace.dirty_files, ", "))
		)
		for _, value in ipairs(workspace.activity) do
			table.insert(lines, "  activity: " .. value.provider .. " · " .. value.session_id .. " · " .. value.state)
		end
		if #workspace.collisions == 0 then
			table.insert(lines, "  writer collisions: none")
		else
			for _, collision in ipairs(workspace.collisions) do
				table.insert(
					lines,
					"  writer collision: "
						.. collision.kind
						.. " · "
						.. collision.path
						.. " · "
						.. table.concat(collision.worktree_ids, ", ")
				)
			end
		end
	end
	table.insert(lines, "j/k navigate · <CR> select · q close · ? help")
	accessibility.render(dashboard.buffer, lines, "gator-workspaces")
end

local function bind(dashboard)
	accessibility.panel(dashboard.buffer, { next = "j", previous = "k", confirm = "<CR>", cancel = "q", help = "?" }, {
		next = function()
			if #dashboard.workspaces > 0 then
				M.select(dashboard.selected % #dashboard.workspaces + 1)
			end
		end,
		previous = function()
			if #dashboard.workspaces > 0 then
				M.select((dashboard.selected - 2) % #dashboard.workspaces + 1)
			end
		end,
		confirm = function()
			if #dashboard.workspaces > 0 then
				M.confirm()
			end
		end,
		cancel = M.close,
		help = function()
			vim.notify("Gator workspaces: j/k navigate, <CR> select, q close", vim.log.levels.INFO)
		end,
	})
end

function M.open(opts)
	if type(opts) ~= "table" then
		fail("open requires options")
	end
	if opts.on_select ~= nil and type(opts.on_select) ~= "function" then
		fail("on_select must be a function")
	end
	local value = workspaces(opts.workspaces or {})
	local dashboard, tabpage = current()
	if dashboard then
		dashboard.workspaces = value
		dashboard.on_select = opts.on_select
		dashboard.selected = math.min(dashboard.selected, math.max(#value, 1))
		render(dashboard)
		vim.api.nvim_set_current_win(dashboard.window)
		return dashboard.window
	end
	local opened = panel_window.open("botright 14new")
	local window = opened.window
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype = "gator-workspaces"
	vim.bo[buffer].bufhidden = "wipe"
	vim.api.nvim_win_set_buf(window, buffer)
	dashboard = {
		window = window,
		buffer = buffer,
		workspaces = value,
		selected = 1,
		on_select = opts.on_select,
		previous = opened.previous,
	}
	dashboards[tabpage] = dashboard
	render(dashboard)
	bind(dashboard)
	return window
end

function M.select(index)
	local dashboard = current()
	if not dashboard then
		fail("no workspace dashboard is open in this tab")
	end
	if type(index) ~= "number" or index % 1 ~= 0 or index < 1 or index > #dashboard.workspaces then
		fail("selection must identify an inspected workspace")
	end
	dashboard.selected = index
	render(dashboard)
	return vim.deepcopy(dashboard.workspaces[index])
end

function M.confirm()
	local dashboard = current()
	if not dashboard or not dashboard.workspaces[dashboard.selected] then
		fail("no workspace is selected")
	end
	local workspace = vim.deepcopy(dashboard.workspaces[dashboard.selected])
	if dashboard.on_select then
		dashboard.on_select(workspace)
	end
	return workspace
end

function M.close()
	local dashboard, tabpage = current()
	if not dashboard then
		return false
	end
	panel_window.close(dashboard.window, dashboard.previous)
	dashboards[tabpage] = nil
	return true
end

return M
