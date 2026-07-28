local accessibility = require("gator.ui.accessibility")
local panel_window = require("gator.ui.window")
local redact = require("gator.policy.redact")
local trust = require("gator.trust")

local M = {}
local panels = {}
local defaults = { layout = "split", height = 18, width = 0 }
local settings = vim.deepcopy(defaults)

local function fail(message)
	error("Gator conversation: " .. redact.text(tostring(message)), 3)
end

local function layout(value)
	if value ~= "split" and value ~= "float" and value ~= "fullscreen" then
		fail("layout must be split, float, or fullscreen")
	end
	return value
end

function M.configure(opts)
	opts = opts or {}
	if type(opts) ~= "table" or (vim.islist(opts) and next(opts) ~= nil) then
		fail("settings must be an object")
	end
	for key in pairs(opts) do
		if key ~= "layout" and key ~= "height" and key ~= "width" then
			fail("settings contain unsupported field: " .. tostring(key))
		end
	end
	local value = vim.tbl_deep_extend("force", vim.deepcopy(defaults), opts)
	layout(value.layout)
	if type(value.height) ~= "number" or value.height % 1 ~= 0 or value.height < 6 then
		fail("height must be an integer of at least 6")
	end
	if
		type(value.width) ~= "number"
		or value.width % 1 ~= 0
		or value.width < 0
		or (value.width > 0 and value.width < 20)
	then
		fail("width must be 0 or an integer of at least 20")
	end
	settings = value
	return vim.deepcopy(settings)
end

local function float_dimensions(height)
	local maximum_width = math.max(vim.o.columns - 4, 1)
	local maximum_height = math.max(vim.o.lines - 4, 1)
	local width = settings.width == 0 and math.floor(vim.o.columns * 0.75) or settings.width
	return math.min(width, maximum_width), math.min(height or settings.height, maximum_height)
end

local function open_window(buffer, mode, previous)
	if mode == "float" then
		local width, height = float_dimensions()
		return vim.api.nvim_open_win(buffer, true, {
			relative = "editor",
			row = math.max(math.floor((vim.o.lines - height) / 2), 0),
			col = math.max(math.floor((vim.o.columns - width) / 2), 0),
			width = width,
			height = height,
			style = "minimal",
			border = "rounded",
		})
	end
	if previous and vim.api.nvim_win_is_valid(previous) then
		vim.api.nvim_set_current_win(previous)
	end
	vim.cmd(mode == "split" and "botright " .. settings.height .. "new" or "botright new")
	local window = vim.api.nvim_get_current_win()
	vim.api.nvim_win_set_buf(window, buffer)
	if mode == "fullscreen" then
		vim.cmd("wincmd _")
		vim.cmd("wincmd |")
	end
	return window
end

local function current()
	local tabpage = vim.api.nvim_get_current_tabpage()
	local panel = panels[tabpage]
	if
		panel
		and vim.api.nvim_win_is_valid(panel.window)
		and vim.api.nvim_buf_is_valid(panel.buffer)
		and vim.api.nvim_win_get_buf(panel.window) == panel.buffer
	then
		return panel, tabpage
	end
	panels[tabpage] = nil
	return nil, tabpage
end

local function elapsed(panel)
	if panel.state ~= "running" or type(panel.turn_started_at) ~= "number" then
		return nil
	end
	local seconds = math.max(0, os.time() - panel.turn_started_at)
	return string.format("%d:%02d", math.floor(seconds / 60), seconds % 60)
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
	if panel.state ~= "running" then
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
			if
				panel.timer ~= timer
				or not vim.api.nvim_win_is_valid(panel.window)
				or not vim.api.nvim_buf_is_valid(panel.buffer)
			then
				stop_timer(panel)
				return
			end
			M.render(panel)
		end)
	)
end

local function markdown_line(line)
	if line == "## user" then
		return "You"
	end
	if line == "## assistant" then
		return "Gator agent"
	end
	if line:match("^```[^`]*$") then
		return "— code —"
	end
	line = line:gsub("%[([^%]]+)%]%b()", "%1")
	line = line:gsub("`([^`]+)`", "%1")
	return line:gsub("^(%s*)#+%s+", "%1")
end

local function display_lines(values)
	local result = {}
	for index, line in ipairs(values) do
		result[index] = markdown_line(line)
	end
	return result
end

local function workspace_label(value)
	if
		type(value) ~= "table"
		or (value.kind ~= "project" and value.kind ~= "worktree")
		or type(value.root) ~= "string"
	then
		return "workspace unavailable"
	end
	return value.kind .. " " .. vim.fn.fnamemodify(value.root, ":t")
end

local function trust_label(value, workspace)
	if value == nil then
		return "Trust: unavailable · " .. workspace_label(workspace)
	end
	local normalized = trust.normalize(value)
	local policy = normalized.policy.mode or normalized.policy.state
	local write = normalized.write.mode or normalized.write.state
	local approval = normalized.approval.state:gsub("_", "-")
	return "Trust: policy "
		.. policy
		.. " · write "
		.. write:gsub("_", "-")
		.. " · approval "
		.. approval
		.. " · "
		.. workspace_label(workspace)
end

function M.render(panel)
	local label = panel.run_id and ("run " .. panel.run_id) or panel.session_id
	local header = "Gator agent · " .. panel.provider .. " · " .. label .. " · " .. panel.state
	if panel.state == "running" then
		header = header
			.. " · "
			.. (panel.cancelling and "cancelling" or panel.phase or "working")
			.. " · "
			.. elapsed(panel)
	end
	local lines = { header, trust_label(panel.trust, panel.workspace), "" }
	if #panel.lines == 0 then
		if panel.state == "running" then
			table.insert(
				lines,
				panel.cancelling and "Cancelling current turn" or "Working · " .. (panel.phase or "working")
			)
		elseif panel.state == "waiting_input" then
			table.insert(lines, "Ready for a prompt")
		else
			table.insert(lines, "No provider output")
		end
	else
		vim.list_extend(lines, display_lines(panel.lines))
	end
	table.insert(lines, "")
	if panel.notice then
		table.insert(lines, panel.notice)
	end
	if panel.cancelling then
		table.insert(lines, "cancelling · q detach · + / - resize · f fullscreen · o layout · ? help")
	elseif panel.state == "running" then
		table.insert(lines, "c cancel · q detach · + / - resize · f fullscreen · o layout · ? help")
	elseif panel.state == "waiting_input" then
		table.insert(lines, "i prompt · q detach · + / - resize · f fullscreen · o layout · ? help")
	else
		table.insert(lines, "q close · + / - resize · f fullscreen · o layout · ? help")
	end
	accessibility.render(panel.buffer, lines, "gator-conversation")
end

local function append_lines(target, value)
	for _, line in ipairs(vim.split(redact.text(value), "\n", { plain = true, trimempty = false })) do
		table.insert(target, line)
	end
end

local function append_fragment(target, value)
	local lines = vim.split(redact.text(value), "\n", { plain = true, trimempty = false })
	if #target == 0 then
		vim.list_extend(target, lines)
		return
	end
	target[#target] = target[#target] .. lines[1]
	for index = 2, #lines do
		table.insert(target, lines[index])
	end
end

local function input(panel)
	if panel.state ~= "waiting_input" then
		panel.notice = "Wait for the current response before sending another prompt"
		M.render(panel)
		return false
	end
	vim.ui.input({ prompt = "Gator prompt: " }, function(value)
		if type(value) == "string" and vim.trim(value) ~= "" then
			panel.notice = nil
			append_lines(panel.lines, "> " .. value)
			panel.on_message("user", value)
			M.render(panel)
			panel.on_input(value)
		end
	end)
	return true
end

local function bind(panel)
	accessibility.panel(panel.buffer, {
		prompt = "i",
		cancel = "c",
		close = "q",
		help = "?",
		grow = "+",
		shrink = "-",
		fullscreen = "f",
		layout = "o",
	}, {
		prompt = function()
			input(panel)
		end,
		cancel = function()
			if panel.state ~= "running" or panel.cancelling then
				panel.notice = "No active turn to cancel"
				M.render(panel)
				return false
			end
			panel.cancelling, panel.notice = true, nil
			M.render(panel)
			if panel.on_cancel() == false then
				panel.cancelling, panel.notice = false, "Provider did not accept cancellation"
				M.render(panel)
				return false
			end
			return true
		end,
		close = M.detach,
		help = function()
			panel.notice = panel.state == "waiting_input" and "i prompts · q detaches" or "c cancels · q detaches"
			M.render(panel)
		end,
		grow = function()
			M.resize(4)
		end,
		shrink = function()
			M.resize(-4)
		end,
		fullscreen = function()
			M.toggle_fullscreen()
		end,
		layout = function()
			M.cycle_layout()
		end,
	})
end

function M.open(opts)
	if type(opts) ~= "table" or type(opts.on_input) ~= "function" or type(opts.on_cancel) ~= "function" then
		fail("open requires input and cancel callbacks")
	end
	if opts.on_detach ~= nil and type(opts.on_detach) ~= "function" then
		fail("on_detach must be a function")
	end
	if opts.on_message ~= nil and type(opts.on_message) ~= "function" then
		fail("on_message must be a function")
	end
	if opts.run_id ~= nil and (type(opts.run_id) ~= "string" or opts.run_id == "") then
		fail("run_id must be non-empty text")
	end
	if opts.history ~= nil and (type(opts.history) ~= "table" or not vim.islist(opts.history)) then
		fail("history must be an array")
	end
	for _, name in ipairs({ "provider", "session_id", "state" }) do
		if type(opts[name]) ~= "string" or opts[name] == "" then
			fail(name .. " must be non-empty text")
		end
	end
	if opts.phase ~= nil and (type(opts.phase) ~= "string" or opts.phase == "") then
		fail("phase must be non-empty text")
	end
	if opts.turn_started_at ~= nil and type(opts.turn_started_at) ~= "number" then
		fail("turn_started_at must be a number")
	end
	if opts.trust ~= nil then
		trust.normalize(opts.trust)
	end
	if opts.workspace ~= nil and type(opts.workspace) ~= "table" then
		fail("workspace must be an object")
	end
	local history = {}
	for _, value in ipairs(opts.history or {}) do
		if type(value) ~= "string" then
			fail("history must contain text")
		end
		append_lines(history, value)
	end
	local panel, tabpage = current()
	if panel then
		panel.provider, panel.session_id, panel.run_id, panel.state =
			opts.provider, opts.session_id, opts.run_id, opts.state
		panel.trust, panel.workspace = opts.trust, opts.workspace
		panel.phase = opts.phase or (opts.state == "running" and "working" or nil)
		panel.turn_started_at = opts.turn_started_at or (opts.state == "running" and os.time() or nil)
		panel.cancelling, panel.notice = false, nil
		panel.lines = history
		panel.on_input, panel.on_cancel, panel.on_detach, panel.on_message =
			opts.on_input, opts.on_cancel, opts.on_detach or function() end, opts.on_message or function() end
		M.render(panel)
		sync_timer(panel)
		vim.api.nvim_set_current_win(panel.window)
		return panel.window
	end
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype, vim.bo[buffer].bufhidden = "gator-conversation", "wipe"
	local previous = vim.api.nvim_get_current_win()
	local mode = settings.layout
	local window = open_window(buffer, mode, previous)
	panel = {
		window = window,
		buffer = buffer,
		layout = mode,
		previous_layout = mode,
		provider = opts.provider,
		session_id = opts.session_id,
		run_id = opts.run_id,
		trust = opts.trust,
		workspace = opts.workspace,
		state = opts.state,
		phase = opts.phase or (opts.state == "running" and "working" or nil),
		turn_started_at = opts.turn_started_at or (opts.state == "running" and os.time() or nil),
		cancelling = false,
		notice = nil,
		lines = history,
		on_input = opts.on_input,
		on_cancel = opts.on_cancel,
		on_detach = opts.on_detach or function() end,
		on_message = opts.on_message or function() end,
		previous = previous,
	}
	panels[tabpage] = panel
	M.render(panel)
	sync_timer(panel)
	bind(panel)
	return panel.window
end

local function replace_layout(panel, mode)
	mode = layout(mode)
	if panel.layout == mode then
		return false
	end
	local old = panel.window
	vim.bo[panel.buffer].bufhidden = "hide"
	if vim.api.nvim_win_is_valid(old) then
		vim.api.nvim_win_close(old, true)
	end
	panel.window = open_window(panel.buffer, mode, panel.previous)
	vim.bo[panel.buffer].bufhidden = "wipe"
	if mode ~= "fullscreen" then
		panel.previous_layout = mode
	end
	panel.layout, panel.notice = mode, nil
	M.render(panel)
	return true
end

function M.resize(delta)
	local panel = current()
	if not panel or type(delta) ~= "number" or delta % 1 ~= 0 or delta == 0 then
		return false
	end
	if panel.layout == "fullscreen" then
		panel.notice = "Fullscreen uses the available editor space"
		M.render(panel)
		return false
	end
	if panel.layout == "float" then
		local config = vim.api.nvim_win_get_config(panel.window)
		local _, maximum = float_dimensions()
		vim.api.nvim_win_set_config(panel.window, { height = math.max(6, math.min(config.height + delta, maximum)) })
	else
		vim.api.nvim_win_set_height(panel.window, math.max(6, vim.api.nvim_win_get_height(panel.window) + delta))
	end
	return true
end

function M.toggle_fullscreen()
	local panel = current()
	if not panel then
		return false
	end
	return replace_layout(panel, panel.layout == "fullscreen" and panel.previous_layout or "fullscreen")
end

function M.cycle_layout()
	local panel = current()
	if not panel then
		return false
	end
	local next_layout = panel.layout == "split" and "float" or (panel.layout == "float" and "fullscreen" or "split")
	return replace_layout(panel, next_layout)
end

function M.update(opts)
	local panel = current()
	if not panel or type(opts) ~= "table" then
		fail("update requires an open panel and options")
	end
	if opts.run_id ~= nil and opts.run_id ~= panel.run_id then
		return false
	end
	if opts.session_id ~= nil then
		panel.session_id = redact.text(opts.session_id)
	end
	if opts.run_id ~= nil then
		panel.run_id = redact.text(opts.run_id)
	end
	if opts.state ~= nil then
		local was_running = panel.state == "running"
		panel.state = redact.text(opts.state)
		if panel.state == "running" and not was_running then
			panel.turn_started_at = opts.turn_started_at or os.time()
		elseif panel.state ~= "running" then
			panel.cancelling, panel.phase = false, nil
		end
	end
	if opts.phase ~= nil then
		panel.phase = redact.text(opts.phase)
	end
	if opts.cancelling ~= nil then
		panel.cancelling = opts.cancelling == true
	end
	if opts.text ~= nil and opts.text ~= "" then
		local value = redact.text(opts.text)
		if opts.append then
			append_fragment(panel.lines, value)
		else
			append_lines(panel.lines, value)
		end
		panel.on_message(opts.role == "user" and "user" or "assistant", value)
	end
	M.render(panel)
	sync_timer(panel)
	return true
end

function M.close()
	local panel, tabpage = current()
	if not panel then
		return false
	end
	stop_timer(panel)
	panel_window.close(panel.window, panel.previous)
	panels[tabpage] = nil
	return true
end

function M.detach()
	local panel = current()
	if not panel then
		return false
	end
	panel.on_detach()
	return M.close()
end

return M
