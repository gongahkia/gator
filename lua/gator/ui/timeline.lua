local M = {}
local timelines = {}
local motion = require("gator.ui.motion")
local approvals = { not_required = true, pending = true, granted = true, denied = true }
local statuses = { pending = true, running = true, succeeded = true, failed = true }

local function fail(message)
	error("Gator timeline: " .. message, 3)
end

local function require_string(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty string")
	end
	return value
end

local function calls(value)
	if type(value) ~= "table" or not vim.islist(value) then
		fail("calls must be an array")
	end
	local result = {}
	local ids = {}
	for index, call in ipairs(value) do
		if type(call) ~= "table" then
			fail("call " .. index .. " must be a table")
		end
		for key in pairs(call) do
			if
				key ~= "id"
				and key ~= "provider"
				and key ~= "session_id"
				and key ~= "name"
				and key ~= "arguments"
				and key ~= "approval"
				and key ~= "output"
				and key ~= "failure"
				and key ~= "status"
			then
				fail("call " .. index .. " contains unsupported field: " .. tostring(key))
			end
		end
		local id = require_string(call.id, "call " .. index .. " id")
		if ids[id] then
			fail("call ids must be unique: " .. id)
		end
		ids[id] = true
		local status = require_string(call.status, "call " .. index .. " status")
		if not statuses[status] then
			fail("call " .. index .. " status is unknown: " .. status)
		end
		local approval = require_string(call.approval, "call " .. index .. " approval")
		if not approvals[approval] then
			fail("call " .. index .. " approval is unknown: " .. approval)
		end
		if call.failure ~= nil and status ~= "failed" then
			fail("only failed calls may provide a failure")
		end
		if status == "failed" and call.failure == nil then
			fail("failed calls must provide a failure")
		end
		result[index] = {
			id = id,
			provider = require_string(call.provider, "call " .. index .. " provider"),
			session_id = require_string(call.session_id, "call " .. index .. " session_id"),
			name = require_string(call.name, "call " .. index .. " name"),
			arguments = require_string(call.arguments, "call " .. index .. " arguments"),
			approval = approval,
			output = call.output ~= nil and require_string(call.output, "call " .. index .. " output") or nil,
			failure = call.failure ~= nil and require_string(call.failure, "call " .. index .. " failure") or nil,
			status = status,
		}
	end
	return result
end

local function current()
	local tabpage = vim.api.nvim_get_current_tabpage()
	local timeline = timelines[tabpage]
	if timeline and vim.api.nvim_win_is_valid(timeline.window) then
		return timeline, tabpage
	end
	timelines[tabpage] = nil
	return nil, tabpage
end

local function detail(lines, label, value)
	for index, line in ipairs(vim.split(value, "\n", { plain = true, trimempty = false })) do
		table.insert(lines, index == 1 and "  " .. label .. ": " .. line or "  " .. line)
	end
end

local function render(timeline)
	local lines = { "Gator tool calls" .. (timeline.action_marker or "") }
	if #timeline.calls == 0 then
		table.insert(lines, "No provider tool calls")
	end
	for _, call in ipairs(timeline.calls) do
		local state = timeline.collapsed[call.id] and "collapsed" or "expanded"
		local status = call.status == "running" and "running " .. timeline.frame or call.status
		table.insert(lines, "[" .. state .. "] " .. call.id .. " · " .. call.name .. " · " .. status)
		table.insert(
			lines,
			"  provider: " .. call.provider .. " · session: " .. call.session_id .. " · approval: " .. call.approval
		)
		if not timeline.collapsed[call.id] then
			detail(lines, "arguments", call.arguments)
			if call.output then
				detail(lines, "output", call.output)
			end
			if call.failure then
				detail(lines, "failure", call.failure)
			end
		end
	end
	vim.api.nvim_buf_set_lines(timeline.buffer, 0, -1, false, lines)
end

local function update_motion(timeline)
	for _, call in ipairs(timeline.calls) do
		if call.status == "running" then
			timeline.status_motion = timeline.status_motion or motion.spinner()
			timeline.status_motion.start(function(frame)
				if not vim.api.nvim_win_is_valid(timeline.window) then
					timeline.status_motion.stop()
					return
				end
				timeline.frame = frame
				render(timeline)
			end)
			return
		end
	end
	if timeline.status_motion then
		timeline.status_motion.stop()
	end
	timeline.frame = ""
end

local function pulse(timeline)
	if timeline.action_motion then
		timeline.action_motion.stop()
	end
	timeline.action_motion = motion.transition({
		from = 1,
		to = 0,
		steps = 1,
		render = function(value)
			if vim.api.nvim_win_is_valid(timeline.window) then
				timeline.action_marker = value > 0 and " •" or ""
				render(timeline)
			end
		end,
	})
end

function M.open(opts)
	if type(opts) ~= "table" then
		fail("open requires options")
	end
	local value = calls(opts.calls or {})
	local timeline, tabpage = current()
	if timeline then
		local collapsed = {}
		for _, call in ipairs(value) do
			collapsed[call.id] = timeline.collapsed[call.id] or false
		end
		timeline.calls = value
		timeline.collapsed = collapsed
		render(timeline)
		update_motion(timeline)
		vim.api.nvim_set_current_win(timeline.window)
		return timeline.window
	end
	vim.cmd("botright 14new")
	local window = vim.api.nvim_get_current_win()
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype = "gator-timeline"
	vim.bo[buffer].bufhidden = "wipe"
	vim.api.nvim_win_set_buf(window, buffer)
	timeline = { window = window, buffer = buffer, calls = value, collapsed = {}, frame = "" }
	timelines[tabpage] = timeline
	render(timeline)
	update_motion(timeline)
	return window
end

function M.update(value)
	local timeline = current()
	if not timeline then
		fail("no tool-call timeline is open in this tab")
	end
	M.open({ calls = value })
end

function M.toggle(id)
	local timeline = current()
	if not timeline then
		fail("no tool-call timeline is open in this tab")
	end
	for _, call in ipairs(timeline.calls) do
		if call.id == id then
			timeline.collapsed[id] = not timeline.collapsed[id]
			pulse(timeline)
			return timeline.collapsed[id]
		end
	end
	fail("call is not present in the timeline: " .. tostring(id))
end

function M.close()
	local timeline, tabpage = current()
	if not timeline then
		return false
	end
	if timeline.status_motion then
		timeline.status_motion.stop()
	end
	if timeline.action_motion then
		timeline.action_motion.stop()
	end
	vim.api.nvim_win_close(timeline.window, true)
	timelines[tabpage] = nil
	return true
end

return M
