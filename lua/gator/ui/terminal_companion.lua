local accessibility = require("gator.ui.accessibility")
local panel_window = require("gator.ui.window")
local redact = require("gator.policy.redact")
local run_state = require("gator.ui.run_state")
local trust = require("gator.trust")

local M = {}
local panels = {}

local function fail(message)
	error("Gator terminal companion: " .. redact.text(tostring(message)), 3)
end

local function current(id)
	local panel = panels[id]
	if panel and vim.api.nvim_win_is_valid(panel.window) and vim.api.nvim_buf_is_valid(panel.buffer) then
		return panel
	end
	panels[id] = nil
	return nil
end

local function active(state)
	return state == "starting" or state == "running" or state == "detached"
end

local function elapsed(panel)
	if type(panel.started_at) ~= "number" then
		return "unknown"
	end
	local seconds = math.max(0, os.time() - panel.started_at)
	return string.format("%d:%02d", math.floor(seconds / 60), seconds % 60)
end

local function trust_label(value)
	if type(value) ~= "table" then
		return "Trust: unavailable"
	end
	local normalized = trust.normalize(value)
	local policy = normalized.policy.mode or normalized.policy.state
	local write = normalized.write.mode or normalized.write.state
	return "Trust: policy " .. policy .. " · write " .. write:gsub("_", "-")
end

local function stop_timer(panel)
	local timer = panel.timer
	panel.timer = nil
	if timer and not timer:is_closing() then
		timer:stop()
		timer:close()
	end
end

local function sync_timer(panel)
	if not active(panel.state) then
		stop_timer(panel)
		return
	end
	if panel.timer and not panel.timer:is_closing() then
		return
	end
	local timer = vim.uv.new_timer()
	panel.timer = timer
	timer:start(
		1000,
		1000,
		vim.schedule_wrap(function()
			if panel.timer ~= timer or not vim.api.nvim_win_is_valid(panel.window) then
				stop_timer(panel)
				return
			end
			M.render(panel)
		end)
	)
end

function M.render(panel)
	local workspace = panel.workspace and vim.fn.fnamemodify(panel.workspace.root, ":~:.") or "unavailable"
	local session = panel.session and panel.session.id and "provider-owned" or "unavailable"
	local lines = {
		"Gator terminal · " .. panel.provider .. " · " .. run_state.summary(panel.state),
		trust_label(panel.trust),
		"Workspace: " .. workspace,
		"Session: " .. session .. " · elapsed " .. elapsed(panel),
		"Transcript: unavailable · terminal output remains provider-owned",
		"Activity: unobservable without terminal scraping",
		"",
	}
	if panel.notice then
		table.insert(lines, panel.notice)
		table.insert(lines, "")
	end
	table.insert(lines, "<CR> focus terminal · s stop · q detach · r runs · l journal · h handoff · ? help")
	accessibility.render(panel.buffer, lines, "gator-terminal-companion")
end

local function bind(panel)
	accessibility.panel(panel.buffer, {
		confirm = "<CR>",
		stop = "s",
		close = "q",
		runs = "r",
		journal = "l",
		handoff = "h",
		help = "?",
	}, {
		confirm = function()
			return panel.on_focus()
		end,
		stop = function()
			return panel.on_stop()
		end,
		close = function()
			panel.on_detach()
			return M.close(panel.run_id)
		end,
		runs = function()
			return panel.on_runs()
		end,
		journal = function()
			return panel.on_journal()
		end,
		handoff = function()
			return panel.on_handoff()
		end,
		help = function()
			panel.notice = "Type prompts in the provider terminal; Gator never reads its output."
			M.render(panel)
		end,
	})
end

local function validate(opts)
	if type(opts) ~= "table" then
		fail("open requires options")
	end
	for _, key in ipairs({ "run_id", "provider", "state" }) do
		if type(opts[key]) ~= "string" or opts[key] == "" then
			fail(key .. " must be non-empty text")
		end
	end
	if type(opts.terminal_window) ~= "number" or not vim.api.nvim_win_is_valid(opts.terminal_window) then
		fail("terminal_window must be valid")
	end
	for _, key in ipairs({ "on_focus", "on_stop", "on_detach", "on_runs", "on_journal", "on_handoff" }) do
		if type(opts[key]) ~= "function" then
			fail(key .. " must be a function")
		end
	end
end

local function assign(panel, opts)
	panel.provider, panel.state = opts.provider, opts.state
	panel.trust, panel.workspace, panel.session = opts.trust, vim.deepcopy(opts.workspace), vim.deepcopy(opts.session)
	panel.started_at = opts.started_at or panel.started_at or os.time()
	panel.on_focus, panel.on_stop, panel.on_detach = opts.on_focus, opts.on_stop, opts.on_detach
	panel.on_runs, panel.on_journal, panel.on_handoff = opts.on_runs, opts.on_journal, opts.on_handoff
	panel.notice = nil
end

function M.open(opts)
	validate(opts)
	local panel = current(opts.run_id)
	if panel then
		assign(panel, opts)
		M.render(panel)
		sync_timer(panel)
		return panel.window
	end
	local previous = opts.terminal_window
	vim.api.nvim_set_current_win(previous)
	local opened = panel_window.open("vertical rightbelow 42new")
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype, vim.bo[buffer].bufhidden = "gator-terminal-companion", "wipe"
	vim.api.nvim_win_set_buf(opened.window, buffer)
	panel = { run_id = opts.run_id, window = opened.window, buffer = buffer, previous = previous }
	assign(panel, opts)
	panels[opts.run_id] = panel
	M.render(panel)
	bind(panel)
	sync_timer(panel)
	if vim.api.nvim_win_is_valid(previous) then
		vim.api.nvim_set_current_win(previous)
	end
	return panel.window
end

function M.update(opts)
	if type(opts) ~= "table" or type(opts.run_id) ~= "string" then
		fail("update requires a run id")
	end
	local panel = current(opts.run_id)
	if not panel then
		return false
	end
	for _, key in ipairs({ "state", "trust", "workspace", "session", "started_at" }) do
		if opts[key] ~= nil then
			panel[key] = vim.deepcopy(opts[key])
		end
	end
	M.render(panel)
	sync_timer(panel)
	return true
end

function M.close(id)
	local panel = current(id)
	if not panel then
		return false
	end
	stop_timer(panel)
	panel_window.close(panel.window, panel.previous)
	panels[id] = nil
	return true
end

function M.inspect(id)
	local panel = current(id)
	if not panel then
		return nil
	end
	return { window = panel.window, buffer = panel.buffer, run_id = panel.run_id }
end

return M
