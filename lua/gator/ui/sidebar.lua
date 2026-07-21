local M = {}
local sidebars = {}
local motion = require("gator.ui.motion")
local accessibility = require("gator.ui.accessibility")
local panel_window = require("gator.ui.window")

local function fail(message)
	error("Gator sidebar: " .. message, 3)
end

local function validate_sessions(sessions)
	if type(sessions) ~= "table" or not vim.islist(sessions) then
		fail("sessions must be an array")
	end
	local result = {}
	for index, session in ipairs(sessions) do
		if type(session) ~= "table" then
			fail("session " .. index .. " must be a table")
		end
		for key in pairs(session) do
			if key ~= "task_id" and key ~= "provider" and key ~= "id" and key ~= "streaming" then
				fail("session " .. index .. " contains unsupported field: " .. tostring(key))
			end
		end
		if type(session.task_id) ~= "string" or session.task_id == "" then
			fail("session " .. index .. " task_id must be a non-empty string")
		end
		if type(session.provider) ~= "string" or session.provider == "" then
			fail("session " .. index .. " provider must be a non-empty string")
		end
		if type(session.id) ~= "string" or session.id == "" then
			fail("session " .. index .. " id must be a non-empty string")
		end
		if type(session.streaming) ~= "boolean" then
			fail("session " .. index .. " streaming must be boolean")
		end
		result[index] = vim.deepcopy(session)
	end
	return result
end

local function current()
	local tabpage = vim.api.nvim_get_current_tabpage()
	local sidebar = sidebars[tabpage]
	if sidebar and vim.api.nvim_win_is_valid(sidebar.window) then
		return sidebar, tabpage
	end
	sidebars[tabpage] = nil
	return nil, tabpage
end

local function render(sidebar)
	local lines = { "Gator conversations" }
	if #sidebar.sessions == 0 then
		table.insert(lines, "No linked sessions")
	else
		for index, session in ipairs(sidebar.sessions) do
			local marker = index == sidebar.selected and ">" or " "
			local status = session.streaming and "streaming " .. sidebar.frame or "idle"
			table.insert(lines, marker .. " " .. session.task_id .. " · " .. session.provider .. " · " .. status)
		end
	end
	table.insert(lines, "j/k navigate · <CR> prompt selected session · q close · ? help")
	accessibility.render(sidebar.buffer, lines, "gator-sidebar")
end

local function bind(sidebar)
	accessibility.panel(sidebar.buffer, { next = "j", previous = "k", prompt = "<CR>", cancel = "q", help = "?" }, {
		next = function()
			if #sidebar.sessions > 0 then
				M.select(sidebar.selected % #sidebar.sessions + 1)
			end
		end,
		previous = function()
			if #sidebar.sessions > 0 then
				M.select((sidebar.selected - 2) % #sidebar.sessions + 1)
			end
		end,
		prompt = function()
			local session = sidebar.sessions[sidebar.selected]
			if session then
				vim.ui.input({ prompt = "Gator prompt: " }, function(text)
					if type(text) == "string" and text ~= "" then
						sidebar.on_input(vim.deepcopy(session), text)
					end
				end)
			end
		end,
		cancel = M.close,
		help = function()
			vim.notify("Gator conversations: j/k navigate, <CR> prompt, q close", vim.log.levels.INFO)
		end,
	})
end

local function update_motion(sidebar)
	for _, session in ipairs(sidebar.sessions) do
		if session.streaming then
			sidebar.stream_motion = sidebar.stream_motion or motion.spinner()
			sidebar.stream_motion.start(function(frame)
				if not vim.api.nvim_win_is_valid(sidebar.window) then
					sidebar.stream_motion.stop()
					return
				end
				sidebar.frame = frame
				render(sidebar)
			end)
			return
		end
	end
	if sidebar.stream_motion then
		sidebar.stream_motion.stop()
	end
	sidebar.frame = ""
end

function M.open(opts)
	if type(opts) ~= "table" or type(opts.on_input) ~= "function" then
		fail("open requires an on_input callback")
	end
	local sessions = validate_sessions(opts.sessions or {})
	local sidebar, tabpage = current()
	if sidebar then
		sidebar.sessions = sessions
		sidebar.on_input = opts.on_input
		sidebar.selected = math.min(sidebar.selected, math.max(#sessions, 1))
		render(sidebar)
		update_motion(sidebar)
		vim.api.nvim_set_current_win(sidebar.window)
		return sidebar.window
	end
	local opened = panel_window.open("topleft vertical 40new")
	local window = opened.window
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype = "gator-sidebar"
	vim.bo[buffer].bufhidden = "wipe"
	vim.api.nvim_win_set_buf(window, buffer)
	sidebar = {
		window = window,
		buffer = buffer,
		sessions = sessions,
		selected = 1,
		on_input = opts.on_input,
		frame = "",
		previous = opened.previous,
	}
	sidebars[tabpage] = sidebar
	render(sidebar)
	bind(sidebar)
	update_motion(sidebar)
	return window
end

function M.select(index)
	local sidebar = current()
	if not sidebar then
		fail("no Gator conversation sidebar is open in this tab")
	end
	if type(index) ~= "number" or index % 1 ~= 0 or index < 1 or index > #sidebar.sessions then
		fail("selection must identify a linked session")
	end
	sidebar.selected = index
	render(sidebar)
	return vim.deepcopy(sidebar.sessions[index])
end

function M.route_input(text)
	local sidebar = current()
	if not sidebar then
		fail("no Gator conversation sidebar is open in this tab")
	end
	if type(text) ~= "string" or text == "" then
		fail("input text must be non-empty")
	end
	local session = sidebar.sessions[sidebar.selected]
	if not session then
		fail("no linked session is selected")
	end
	sidebar.on_input(vim.deepcopy(session), text)
end

function M.close()
	local sidebar, tabpage = current()
	if not sidebar then
		return false
	end
	if sidebar.stream_motion then
		sidebar.stream_motion.stop()
	end
	panel_window.close(sidebar.window, sidebar.previous)
	sidebars[tabpage] = nil
	return true
end

return M
