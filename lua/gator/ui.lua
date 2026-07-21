local core_task = require("gator.core.task")
local core_run = require("gator.core.run")
local context_pack = require("gator.context.pack")
local state_store = require("gator.state")
local M = {
	name = "ui",
	api_version = 1,
	context_inspector = require("gator.ui.context_inspector"),
	dashboard = require("gator.ui.dashboard"),
	diff_review = require("gator.ui.diff_review"),
	event_details = require("gator.ui.event_details"),
	escalation = require("gator.ui.escalation"),
	accessibility = require("gator.ui.accessibility"),
	markdown = require("gator.ui.markdown"),
	palette = require("gator.ui.palette"),
	picker = require("gator.ui.picker"),
	provider_picker = require("gator.ui.provider_picker"),
	selection = require("gator.ui.selection"),
	session_picker = require("gator.ui.session_picker"),
	task_file_picker = require("gator.ui.task_file_picker"),
	sidebar = require("gator.ui.sidebar"),
	timeline = require("gator.ui.timeline"),
	workspace_dashboard = require("gator.ui.workspace_dashboard"),
	motion = require("gator.ui.motion"),
}
local panels = {}
local accessibility = require("gator.ui.accessibility")
local statuses = { ready = true, loading = true, failed = true, recovering = true }

local function fail(message)
	error("Gator UI: " .. message, 3)
end

local function current_panel()
	local tabpage = vim.api.nvim_get_current_tabpage()
	local panel = panels[tabpage]
	if panel and vim.api.nvim_win_is_valid(panel.window) then
		return panel, tabpage
	end
	panels[tabpage] = nil
	return nil, tabpage
end

local function height()
	return math.max(12, math.min(24, math.floor(vim.o.lines * 0.5)))
end

local function workspace_state(state)
	if type(state.workspace) ~= "table" or not statuses[state.workspace.status] then
		fail("workspace state must expose ready, loading, failed, or recovering status")
	end
	if state.workspace.detail ~= nil and (type(state.workspace.detail) ~= "string" or state.workspace.detail == "") then
		fail("workspace detail must be a non-empty string")
	end
	return state.workspace
end

local function tasks(state)
	if type(state.tasks) ~= "table" or not vim.islist(state.tasks) then
		fail("workspace tasks must be an array")
	end
	local result = {}
	for index, value in ipairs(state.tasks) do
		if not core_task.is(value) then
			fail("workspace task " .. index .. " must be a Gator task")
		end
		result[index] = core_task.to_record(value)
	end
	return result
end

local function task_rows(state)
	local result = {}
	for _, value in ipairs(tasks(state)) do
		table.insert(result, {
			id = value.id,
			objective = value.objective,
			lifecycle = value.lifecycle,
			provider = value.sessions[1] and value.sessions[1].provider or "unassigned",
			workspace = value.workspace and value.workspace.kind or "unassigned",
			review_state = value.lifecycle == "awaiting_review" and "ready" or "unavailable",
		})
	end
	return result
end

local function sessions(state)
	local result = {}
	for _, value in ipairs(tasks(state)) do
		for _, session in ipairs(value.sessions) do
			table.insert(result, {
				task_id = value.id,
				provider = session.provider,
				id = session.id,
				streaming = value.lifecycle == "running",
			})
		end
	end
	return result
end

local function context_entries(state)
	if state.context.selections == nil then
		return {}
	end
	if type(state.context.selections) ~= "table" or not vim.islist(state.context.selections) then
		fail("workspace selections must be an array")
	end
	local result = {}
	for index, selection in ipairs(state.context.selections) do
		if type(selection) ~= "table" then
			fail("workspace selection " .. index .. " must retain a context entry")
		end
		table.insert(result, context_pack.entry(selection.entry))
	end
	return result
end

local function providers(state)
	if type(state.adapters) ~= "table" then
		fail("workspace adapters must be a table")
	end
	local lines = {}
	for name, value in pairs(state.adapters) do
		if type(name) ~= "string" or name == "" or type(value) ~= "table" or type(value.available) ~= "boolean" then
			fail("workspace provider state is invalid")
		end
		if value.available then
			table.insert(lines, "Provider " .. name .. ": available")
		else
			local reason = type(value.reason) == "string" and value.reason ~= "" and value.reason
				or "no readiness detail"
			table.insert(lines, "Provider " .. name .. ": unavailable · " .. reason)
		end
	end
	table.sort(lines)
	return lines
end

local function review_state(state)
	if type(state.review) ~= "table" then
		fail("workspace review state must be a table")
	end
	if core_run.is(state.review.run) and type(state.review.changes) == "table" and vim.islist(state.review.changes) then
		return "ready · " .. #state.review.changes .. " change(s)"
	end
	return "unavailable · no reviewed run evidence"
end

local function render(panel)
	local state = panel.state
	local workspace = workspace_state(state)
	local task_values, session_values, entries = tasks(state), sessions(state), context_entries(state)
	local lines =
		{ "Gator workspace", "State: " .. workspace.status .. (workspace.detail and " · " .. workspace.detail or "") }
	if #task_values == 0 then
		table.insert(lines, "Tasks: empty · create or import a task to begin")
	else
		table.insert(lines, "Tasks: " .. #task_values .. " · open task dashboard")
		for _, value in ipairs(task_values) do
			table.insert(lines, "  " .. value.id .. " · " .. value.lifecycle .. " · " .. value.objective)
		end
	end
	table.insert(
		lines,
		#session_values == 0 and "Sessions: empty · no provider-native sessions linked"
			or "Sessions: " .. #session_values .. " linked provider-native session(s)"
	)
	table.insert(
		lines,
		#entries == 0 and "Context: empty · capture a selection to add provenance"
			or "Context: " .. #entries .. " captured provenance entry/entries"
	)
	table.insert(lines, "Review: " .. review_state(state))
	local provider_lines = providers(state)
	if #provider_lines == 0 then
		table.insert(lines, "Providers: loading/unavailable · run health to verify adapters")
	else
		vim.list_extend(lines, provider_lines)
	end
	table.insert(lines, "")
	table.insert(lines, "Actions:")
	local actions = {
		"Open task dashboard",
		"Open linked sessions",
		"Inspect captured context",
		"Open review evidence",
		"Run health / recover providers",
		"Close workspace",
	}
	for index, action in ipairs(actions) do
		table.insert(lines, (panel.selected == index and "> " or "  ") .. action)
	end
	table.insert(lines, "<CR> confirm · j/k navigate · q close · ? help")
	accessibility.render(panel.buffer, lines, "gator")
end

local function open_dashboard(panel)
	M.dashboard.open({
		tasks = task_rows(panel.state),
		on_open = function(task)
			M.set_status(panel.state, "ready", "selected task " .. task.id)
		end,
	})
end

local function open_sessions(panel)
	M.sidebar.open({
		sessions = sessions(panel.state),
		on_input = function(session)
			M.set_status(panel.state, "ready", "input routed to " .. session.provider .. " session " .. session.id)
		end,
	})
end

local function inspect_context(panel)
	local task_values = tasks(panel.state)
	local task_id = task_values[1] and task_values[1].id or "workspace"
	M.context_inspector.open({
		pack = context_pack.new({ id = "workspace-context", task_id = task_id, entries = context_entries(panel.state) }),
		on_confirm = function(selected)
			M.set_status(panel.state, "ready", "confirmed " .. #selected.entries .. " context entries")
		end,
	})
end

local function open_review(panel)
	local review = panel.state.review
	if not (core_run.is(review.run) and type(review.changes) == "table" and vim.islist(review.changes)) then
		M.set_status(panel.state, "failed", "review evidence is unavailable")
		return
	end
	M.diff_review.open({ run = review.run, changes = review.changes })
end

local function recover(panel)
	M.set_status(panel.state, "recovering", "opening health checks")
	vim.cmd("checkhealth gator")
end

local function bind(panel)
	accessibility.panel(panel.buffer, { next = "j", previous = "k", confirm = "<CR>", cancel = "q", help = "?" }, {
		next = function()
			panel.selected = panel.selected % 6 + 1
			render(panel)
		end,
		previous = function()
			panel.selected = panel.selected == 1 and 6 or panel.selected - 1
			render(panel)
		end,
		confirm = function()
			({ open_dashboard, open_sessions, inspect_context, open_review, recover, M.close })[panel.selected](panel)
		end,
		cancel = M.close,
		help = function()
			vim.notify(
				"Gator workspace: j/k actions, <CR> open, q close; task, session, context, review, and recovery actions are listed",
				vim.log.levels.INFO
			)
		end,
	})
end

function M.set_status(state, status, detail)
	if
		type(state) ~= "table"
		or not statuses[status]
		or (detail ~= nil and (type(detail) ~= "string" or detail == ""))
	then
		fail("status requires initialized state, a known status, and optional detail")
	end
	if not state_store.is(state) then
		fail("status requires a reactive Gator state store")
	end
	return state:update({ workspace = { status = status, detail = detail } }).workspace
end

function M.create_task(state, opts)
	if not state_store.is(state) or type(opts) ~= "table" then
		fail("create_task requires initialized Gator state and options")
	end
	for key in pairs(opts) do
		if key ~= "id" and key ~= "objective" and key ~= "captures" and key ~= "now" then
			fail("create_task contains unsupported field: " .. tostring(key))
		end
	end
	if type(opts.id) ~= "string" or not opts.id:match("^[a-z][a-z0-9_-]*$") then
		fail("create_task id must be a lowercase identifier")
	end
	if type(opts.objective) ~= "string" or opts.objective == "" then
		fail("create_task objective must be non-empty text")
	end
	local now = opts.now or os.time()
	if type(now) ~= "number" or now < 0 or now % 1 ~= 0 then
		fail("create_task now must be a non-negative integer")
	end
	local captures = opts.captures or {}
	if type(captures) ~= "table" or not vim.islist(captures) then
		fail("create_task captures must be an array")
	end
	local evidence = {}
	for index, capture in ipairs(captures) do
		local entry = context_pack.entry(capture)
		evidence[index] = { kind = entry.kind, ref = entry.ref }
	end
	local task = core_task.new({
		id = opts.id,
		objective = opts.objective,
		lifecycle = "draft",
		evidence = evidence,
		created_at = now,
		updated_at = now,
	})
	state:mutate(function(next)
		for _, existing in ipairs(next.tasks) do
			if core_task.is(existing) and existing.id == task.id then
				fail("create_task id is already present")
			end
		end
		table.insert(next.tasks, task)
	end)
	return core_task.to_record(task)
end

function M.prompt_task(state, opts)
	if not state_store.is(state) or type(opts) ~= "table" or type(opts.on_created) ~= "function" then
		fail("prompt_task requires initialized Gator state, options, and callback")
	end
	for key in pairs(opts) do
		if key ~= "id" and key ~= "captures" and key ~= "now" and key ~= "on_created" then
			fail("prompt_task contains unsupported field: " .. tostring(key))
		end
	end
	vim.ui.input({ prompt = "Gator task: " }, function(objective)
		if type(objective) == "string" and objective ~= "" then
			opts.on_created(M.create_task(state, {
				id = opts.id,
				objective = objective,
				captures = opts.captures,
				now = opts.now,
			}))
		end
	end)
end

local function subscribe(panel)
	if not state_store.is(panel.state) then
		return
	end
	panel.subscription = panel.state:subscribe(function()
		local current = current_panel()
		if current and current == panel and current.state == panel.state then
			render(panel)
		end
	end)
end

local function unsubscribe(panel)
	if panel.subscription then
		panel.subscription:cancel()
		panel.subscription = nil
	end
end

function M.open(state)
	if not state_store.is(state) or type(state.config) ~= "table" or type(state.config.context) ~= "table" then
		fail("open requires initialized Gator state")
	end
	accessibility.configure(state.config.ui)
	workspace_state(state)
	local panel, tabpage = current_panel()
	if panel then
		unsubscribe(panel)
		panel.state = state
		subscribe(panel)
		render(panel)
		vim.api.nvim_set_current_win(panel.window)
		return panel.window
	end
	local previous = vim.api.nvim_get_current_win()
	vim.cmd("botright 1new")
	local window = vim.api.nvim_get_current_win()
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype = "gator"
	vim.bo[buffer].bufhidden = "wipe"
	vim.api.nvim_win_set_buf(window, buffer)
	vim.api.nvim_win_set_height(window, height())
	panel = { window = window, buffer = buffer, previous = previous, state = state, selected = 1 }
	panels[tabpage] = panel
	subscribe(panel)
	render(panel)
	bind(panel)
	return window
end

function M.focus()
	local panel = current_panel()
	if not panel then
		fail("no Gator panel is open in this tab")
	end
	vim.api.nvim_set_current_win(panel.window)
	return panel.window
end

function M.resize(lines)
	local panel = current_panel()
	if not panel then
		fail("no Gator panel is open in this tab")
	end
	if type(lines) ~= "number" or lines < 1 or lines % 1 ~= 0 then
		fail("panel height must be a positive integer")
	end
	vim.api.nvim_win_set_height(panel.window, lines)
	return vim.api.nvim_win_get_height(panel.window)
end

function M.close()
	local panel, tabpage = current_panel()
	if not panel then
		return false
	end
	local previous = panel.previous
	unsubscribe(panel)
	vim.api.nvim_win_close(panel.window, true)
	panels[tabpage] = nil
	if previous and vim.api.nvim_win_is_valid(previous) then
		vim.api.nvim_set_current_win(previous)
	end
	return true
end

function M.restore(state)
	local panel = current_panel()
	if panel then
		return M.focus()
	end
	return M.open(state)
end

return M
