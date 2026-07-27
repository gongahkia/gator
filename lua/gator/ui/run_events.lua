local accessibility = require("gator.ui.accessibility")
local panel_window = require("gator.ui.window")

local M = {}
local panels = {}

local function fail(message)
	error("Gator run events: " .. tostring(message), 3)
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

local function timestamp(value)
	return os.date("%Y-%m-%d %H:%M:%S", value)
end

local function render(panel)
	local lines = { "Gator event journal · " .. panel.run.id, "" }
	if #panel.events == 0 then
		table.insert(lines, "No Gator-owned events were recorded for this legacy run.")
	else
		for index, event in ipairs(panel.events) do
			local selected = index == panel.selected and "> " or "  "
			table.insert(lines, selected .. timestamp(event.at) .. " · " .. event.type)
			if panel.expanded[event.sequence] then
				for _, line in
					ipairs(vim.split(vim.json.encode(event.payload), "\n", { plain = true, trimempty = false }))
				do
					table.insert(lines, "  " .. line)
				end
			end
		end
	end
	table.insert(lines, "")
	table.insert(lines, "j/k navigate · <CR> details · q close · ? help")
	accessibility.render(panel.buffer, lines, "gator-run-events")
end

local function bind(panel)
	accessibility.panel(panel.buffer, { next = "j", previous = "k", confirm = "<CR>", close = "q", help = "?" }, {
		next = function()
			if #panel.events > 0 then
				panel.selected = panel.selected % #panel.events + 1
				render(panel)
			end
		end,
		previous = function()
			if #panel.events > 0 then
				panel.selected = (panel.selected - 2) % #panel.events + 1
				render(panel)
			end
		end,
		confirm = function()
			local event = panel.events[panel.selected]
			if event then
				panel.expanded[event.sequence] = not panel.expanded[event.sequence]
				render(panel)
			end
		end,
		close = M.close,
		help = function()
			vim.notify("Gator journal: j/k navigate, <CR> show metadata, q close", vim.log.levels.INFO)
		end,
	})
end

function M.open(opts)
	if type(opts) ~= "table" or type(opts.run) ~= "table" or type(opts.run.id) ~= "string" then
		fail("open requires a run")
	end
	if type(opts.events) ~= "table" or not vim.islist(opts.events) then
		fail("open requires an event array")
	end
	local events = vim.deepcopy(opts.events)
	for index, event in ipairs(events) do
		if
			type(event) ~= "table"
			or type(event.sequence) ~= "number"
			or type(event.type) ~= "string"
			or type(event.at) ~= "number"
			or type(event.payload) ~= "table"
		then
			fail("event " .. index .. " is invalid")
		end
	end
	local panel, tabpage = current()
	if panel then
		panel.run, panel.events = vim.deepcopy(opts.run), events
		panel.selected = math.min(panel.selected, math.max(#events, 1))
		render(panel)
		vim.api.nvim_set_current_win(panel.window)
		return panel.window
	end
	local opened = panel_window.open("botright 18new")
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype, vim.bo[buffer].bufhidden = "gator-run-events", "wipe"
	vim.api.nvim_win_set_buf(opened.window, buffer)
	panel = {
		window = opened.window,
		buffer = buffer,
		previous = opened.previous,
		run = vim.deepcopy(opts.run),
		events = events,
		expanded = {},
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
