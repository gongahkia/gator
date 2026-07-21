local M = {}
local timelines = {}
local motion = require("gator.ui.motion")
local accessibility = require("gator.ui.accessibility")
local redact = require("gator.policy.redact")
local approvals = { not_required = true, pending = true, granted = true, denied = true }
local statuses = { pending = true, running = true, succeeded = true, failed = true }
local max_output_lines = 200
local max_output_bytes = 65536

local function fail(message)
	error("Gator timeline: " .. message, 3)
end

local function require_string(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty string")
	end
	return value
end

local function bounded_output(value, name)
	value = require_string(value, name)
	value = redact.text(value)
	local lines, offset, retained = {}, 1, 0
	while offset <= #value and #lines < max_output_lines and retained < max_output_bytes do
		local ending = value:find("\n", offset, true)
		local line = value:sub(offset, ending and ending - 1 or #value)
		local remaining = max_output_bytes - retained
		if #line > remaining then
			line = line:sub(1, remaining)
			ending = nil
		end
		table.insert(lines, line)
		retained = retained + #line + 1
		if not ending then
			break
		end
		offset = ending + 1
	end
	local result = table.concat(lines, "\n")
	if offset <= #value then
		local marker = "[output truncated in Gator; inspect the provider-native session for full history]"
		if #result + #marker + 1 > max_output_bytes then
			result = result:sub(1, math.max(0, max_output_bytes - #marker - 1))
		end
		result = result == "" and marker or result .. "\n" .. marker
	end
	return result
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
			arguments = redact.text(require_string(call.arguments, "call " .. index .. " arguments")),
			approval = approval,
			output = call.output ~= nil and bounded_output(call.output, "call " .. index .. " output") or nil,
			failure = call.failure ~= nil and bounded_output(call.failure, "call " .. index .. " failure") or nil,
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
	local values = vim.split(value, "\n", { plain = true, trimempty = false })
	for index, line in ipairs(values) do
		if index > max_output_lines then
			table.insert(lines, "  output truncated in Gator; inspect the provider-native session for full history")
			break
		end
		table.insert(lines, index == 1 and "  " .. label .. ": " .. line or "  " .. line)
	end
end

local function focused(timeline)
	return vim.api.nvim_win_is_valid(timeline.window)
		and vim.api.nvim_get_current_tabpage() == timeline.tabpage
		and vim.api.nvim_get_current_win() == timeline.window
end

local function status_line(timeline, call)
	local state = timeline.collapsed[call.id] and "collapsed" or "expanded"
	local status = call.status == "running" and "running " .. timeline.frame or call.status
	local selected = timeline.calls[timeline.selected] and timeline.calls[timeline.selected].id == call.id and "> "
		or "  "
	return selected .. "[" .. state .. "] " .. call.id .. " · " .. call.name .. " · " .. status
end

local function render(timeline)
	local lines = { "Gator tool calls" .. (timeline.action_marker or "") }
	timeline.status_lines = {}
	if #timeline.calls == 0 then
		table.insert(lines, "No provider tool calls")
	end
	for _, call in ipairs(timeline.calls) do
		table.insert(lines, status_line(timeline, call))
		timeline.status_lines[call.id] = #lines
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
	if #lines < 205 then
		table.insert(lines, "j/k navigate · <Space> collapse/expand · q close · ? help")
	end
	accessibility.render(timeline.buffer, lines, "gator-timeline")
end

local function render_status(timeline)
	vim.bo[timeline.buffer].modifiable = true
	for _, call in ipairs(timeline.calls) do
		if call.status == "running" then
			local line = timeline.status_lines and timeline.status_lines[call.id]
			if line then
				vim.api.nvim_buf_set_lines(timeline.buffer, line - 1, line, false, { status_line(timeline, call) })
			end
		end
	end
	vim.bo[timeline.buffer].modifiable = false
end

local function update_motion(timeline)
	if not focused(timeline) then
		if timeline.status_motion then
			timeline.status_motion.stop()
		end
		if timeline.action_motion then
			timeline.action_motion.stop()
		end
		return
	end
	for _, call in ipairs(timeline.calls) do
		if call.status == "running" then
			timeline.status_motion = timeline.status_motion or motion.spinner()
			timeline.status_motion.start(function(frame)
				if not focused(timeline) then
					timeline.status_motion.stop()
					return
				end
				timeline.frame = frame
				render_status(timeline)
			end)
			return
		end
	end
	if timeline.status_motion then
		timeline.status_motion.stop()
	end
	timeline.frame = ""
end

local function replace(timeline, calls)
	local collapsed = {}
	for _, call in ipairs(calls) do
		collapsed[call.id] = timeline.collapsed[call.id] or false
	end
	timeline.calls = calls
	timeline.collapsed = collapsed
	timeline.selected = math.min(timeline.selected or 1, math.max(#calls, 1))
end

local function bind(timeline)
	accessibility.panel(timeline.buffer, { next = "j", previous = "k", toggle = "<Space>", cancel = "q", help = "?" }, {
		next = function()
			if #timeline.calls > 0 then
				timeline.selected = timeline.selected % #timeline.calls + 1
				render(timeline)
			end
		end,
		previous = function()
			if #timeline.calls > 0 then
				timeline.selected = (timeline.selected - 2) % #timeline.calls + 1
				render(timeline)
			end
		end,
		toggle = function()
			local call = timeline.calls[timeline.selected]
			if call then
				M.toggle(call.id)
			end
		end,
		cancel = M.close,
		help = function()
			vim.notify("Gator timeline: j/k navigate, <Space> collapse or expand, q close", vim.log.levels.INFO)
		end,
	})
end

local function schedule_render(timeline)
	if timeline.refresh_pending then
		return
	end
	timeline.refresh_pending = true
	vim.schedule(function()
		timeline.refresh_pending = false
		if vim.api.nvim_win_is_valid(timeline.window) then
			render(timeline)
			update_motion(timeline)
		end
	end)
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
		replace(timeline, value)
		render(timeline)
		vim.api.nvim_set_current_win(timeline.window)
		update_motion(timeline)
		return timeline.window
	end
	vim.cmd("botright 14new")
	local window = vim.api.nvim_get_current_win()
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype = "gator-timeline"
	vim.bo[buffer].bufhidden = "wipe"
	vim.api.nvim_win_set_buf(window, buffer)
	timeline =
		{ window = window, buffer = buffer, calls = value, collapsed = {}, frame = "", tabpage = tabpage, selected = 1 }
	timelines[tabpage] = timeline
	replace(timeline, value)
	render(timeline)
	bind(timeline)
	update_motion(timeline)
	return window
end

function M.update(value)
	local timeline = current()
	if not timeline then
		fail("no tool-call timeline is open in this tab")
	end
	replace(timeline, calls(value))
	schedule_render(timeline)
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

function M.inspect()
	local timeline = current()
	if not timeline then
		return nil
	end
	local output_bytes = 0
	for _, call in ipairs(timeline.calls) do
		output_bytes = output_bytes + #(call.output or "") + #(call.failure or "")
	end
	return {
		motion_active = timeline.status_motion and timeline.status_motion.active or false,
		refresh_pending = timeline.refresh_pending == true,
		output_bytes = output_bytes,
		max_output_bytes = max_output_bytes,
	}
end

function M.refresh()
	for tabpage, timeline in pairs(timelines) do
		if not vim.api.nvim_win_is_valid(timeline.window) then
			timelines[tabpage] = nil
		else
			update_motion(timeline)
		end
	end
end

local group = vim.api.nvim_create_augroup("GatorTimelineActivity", { clear = true })
vim.api.nvim_create_autocmd({ "WinEnter", "WinLeave", "TabEnter", "TabLeave" }, {
	group = group,
	callback = function()
		vim.schedule(M.refresh)
	end,
})

return M
