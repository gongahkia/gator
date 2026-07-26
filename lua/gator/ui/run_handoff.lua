local accessibility = require("gator.ui.accessibility")
local panel_window = require("gator.ui.window")
local redact = require("gator.policy.redact")

local M = {}
local panels = {}

local function fail(message)
	error("Gator run handoff: " .. redact.text(tostring(message)), 3)
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
		"Gator handoff review",
		"Source: " .. panel.source.provider .. " · target: " .. panel.target .. " · profile: " .. panel.profile,
		"Transcript: " .. panel.source.transcript,
		"",
	}
	vim.list_extend(lines, vim.split(panel.body, "\n", { plain = true, trimempty = false }))
	table.insert(lines, "")
	table.insert(lines, "<CR> launch new session · e edit note/bundle · q cancel · ? help")
	accessibility.render(panel.buffer, lines, "gator-handoff")
end

function M.open(opts)
	if type(opts) ~= "table" or type(opts.source) ~= "table" or type(opts.target) ~= "string" or type(opts.body) ~= "string" or type(opts.on_confirm) ~= "function" then
		fail("open requires source, target, body, and confirm callback")
	end
	local panel, tabpage = current()
	if panel then
		panel.source, panel.target, panel.profile, panel.body, panel.on_confirm = opts.source, opts.target, opts.profile, opts.body, opts.on_confirm
		render(panel)
		vim.api.nvim_set_current_win(panel.window)
		return panel.window
	end
	local opened = panel_window.open("botright 18new")
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype, vim.bo[buffer].bufhidden = "gator-handoff", "wipe"
	vim.api.nvim_win_set_buf(opened.window, buffer)
	panel = {
		window = opened.window,
		buffer = buffer,
		previous = opened.previous,
		source = vim.deepcopy(opts.source),
		target = opts.target,
		profile = opts.profile,
		body = redact.text(opts.body),
		on_confirm = opts.on_confirm,
	}
	panels[tabpage] = panel
	render(panel)
	accessibility.panel(buffer, { confirm = "<CR>", edit = "e", cancel = "q", help = "?" }, {
		confirm = function()
			panel.on_confirm(panel.body)
			M.close()
		end,
		edit = function()
			vim.ui.input({ prompt = "Gator handoff note/bundle: ", default = panel.body }, function(value)
				if type(value) == "string" and vim.trim(value) ~= "" then
					panel.body = redact.text(value)
					render(panel)
				end
			end)
		end,
		cancel = M.close,
		help = function()
			vim.notify("Gator handoff: <CR> launch, e edit, q cancel", vim.log.levels.INFO)
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
