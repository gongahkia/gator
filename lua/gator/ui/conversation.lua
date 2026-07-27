local accessibility = require("gator.ui.accessibility")
local panel_window = require("gator.ui.window")
local redact = require("gator.policy.redact")

local M = {}
local panels = {}

local function fail(message)
	error("Gator conversation: " .. redact.text(tostring(message)), 3)
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
	local label = panel.run_id and ("run " .. panel.run_id) or panel.session_id
	local lines = { "Gator agent · " .. panel.provider .. " · " .. label .. " · " .. panel.state, "" }
	if #panel.lines == 0 then
		table.insert(lines, "Waiting for provider output")
	else
		vim.list_extend(lines, panel.lines)
	end
	table.insert(lines, "")
	table.insert(lines, "i prompt · c cancel · q detach · ? help")
	accessibility.render(panel.buffer, lines, "gator-conversation")
end

local function append_lines(target, value)
	for _, line in ipairs(vim.split(redact.text(value), "\n", { plain = true, trimempty = false })) do
		table.insert(target, line)
	end
end

local function input(panel)
	vim.ui.input({ prompt = "Gator prompt: " }, function(value)
		if type(value) == "string" and vim.trim(value) ~= "" then
			append_lines(panel.lines, "> " .. value)
			panel.on_message("user", value)
			render(panel)
			panel.on_input(value)
		end
	end)
end

local function bind(panel)
	accessibility.panel(panel.buffer, { prompt = "i", cancel = "c", close = "q", help = "?" }, {
		prompt = function()
			input(panel)
		end,
		cancel = function()
			panel.on_cancel()
		end,
		close = M.detach,
		help = function()
			vim.notify("Gator agent: i prompt, c cancel, q detach", vim.log.levels.INFO)
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
		panel.lines = history
		panel.on_input, panel.on_cancel, panel.on_detach, panel.on_message =
			opts.on_input, opts.on_cancel, opts.on_detach or function() end, opts.on_message or function() end
		render(panel)
		vim.api.nvim_set_current_win(panel.window)
		return panel.window
	end
	local opened = panel_window.open("botright 18new")
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype, vim.bo[buffer].bufhidden = "gator-conversation", "wipe"
	vim.api.nvim_win_set_buf(opened.window, buffer)
	panel = {
		window = opened.window,
		buffer = buffer,
		provider = opts.provider,
		session_id = opts.session_id,
		run_id = opts.run_id,
		state = opts.state,
		lines = history,
		on_input = opts.on_input,
		on_cancel = opts.on_cancel,
		on_detach = opts.on_detach or function() end,
		on_message = opts.on_message or function() end,
		previous = opened.previous,
	}
	panels[tabpage] = panel
	render(panel)
	bind(panel)
	return panel.window
end

function M.update(opts)
	local panel = current()
	if not panel or type(opts) ~= "table" then
		fail("update requires an open panel and options")
	end
	if opts.session_id ~= nil then
		panel.session_id = redact.text(opts.session_id)
	end
	if opts.run_id ~= nil then
		panel.run_id = redact.text(opts.run_id)
	end
	if opts.state ~= nil then
		panel.state = redact.text(opts.state)
	end
	if opts.text ~= nil and opts.text ~= "" then
		local value = redact.text(opts.text)
		append_lines(panel.lines, value)
		panel.on_message("assistant", value)
	end
	render(panel)
	return true
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

function M.detach()
	local panel = current()
	if not panel then
		return false
	end
	panel.on_detach()
	return M.close()
end

return M
