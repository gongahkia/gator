local provider_event = require("gator.core.provider_event")
local redact = require("gator.policy.redact")
local accessibility = require("gator.ui.accessibility")
local M = {}
local panels = {}
local states = { ready = true, unavailable = true, failed = true }
local types = { ["usage.update"] = true, ["context.compacted"] = true }

local function fail(message)
	error("Gator usage details: " .. redact.text(tostring(message)), 3)
end

local function text(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty string")
	end
	return redact.text(value)
end

local function count(value, name)
	if value ~= nil and (type(value) ~= "number" or value < 0 or value % 1 ~= 0) then
		fail(name .. " must be a non-negative integer")
	end
	return value
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

local function event(value)
	if not provider_event.is(value) then
		fail("events must contain normalized provider events")
	end
	if value.type == "usage.update" then
		local payload = value.payload
		return {
			event = provider_event.from_record(provider_event.to_record(value)),
			kind = "usage",
			input = count(payload.input, "usage input"),
			output = count(payload.output, "usage output"),
			total = count(payload.total, "usage total"),
		}
	end
	local payload = value.payload
	local before = count(payload.before, "context before")
	local after = count(payload.after, "context after")
	if before == nil or after == nil or after > before then
		fail("compaction must reduce or retain context tokens")
	end
	if type(payload.summary) ~= "string" then
		fail("compaction summary must be text")
	end
	return {
		event = provider_event.from_record(provider_event.to_record(value)),
		kind = "compaction",
		before = before,
		after = after,
		summary = redact.text(payload.summary),
	}
end

local function events(value)
	if type(value) ~= "table" or not vim.islist(value) then
		fail("events must be an array")
	end
	local result = {}
	for _, value in ipairs(value) do
		if not provider_event.is(value) then
			fail("events must contain normalized provider events")
		end
		if types[value.type] then
			table.insert(result, event(value))
		end
	end
	table.sort(result, function(left, right)
		local left_event, right_event = left.event, right.event
		return left_event.sequence == right_event.sequence and left_event.id < right_event.id
			or left_event.sequence < right_event.sequence
	end)
	return result
end

local function value(opts)
	if type(opts) ~= "table" then
		fail("open requires options")
	end
	for key in pairs(opts) do
		if key ~= "events" and key ~= "state" and key ~= "reason" and key ~= "on_cancel" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	if opts.on_cancel ~= nil and type(opts.on_cancel) ~= "function" then
		fail("on_cancel must be a function")
	end
	local result = events(opts.events or {})
	local state = opts.state or (#result == 0 and "unavailable" or "ready")
	if not states[state] then
		fail("state must be ready, unavailable, or failed")
	end
	if state == "ready" and #result == 0 then
		fail("ready state requires usage or compaction events")
	end
	if state ~= "ready" and #result > 0 then
		fail("unavailable or failed state must not contain events")
	end
	local detail_reason = opts.reason
	if state ~= "ready" and detail_reason == nil then
		detail_reason = state == "unavailable" and "usage and context state are unavailable"
			or "usage state rendering failed"
	end
	if detail_reason ~= nil then
		detail_reason = text(detail_reason, "reason")
	end
	return { events = result, state = state, reason = detail_reason, on_cancel = opts.on_cancel }
end

local function render(panel)
	local lines = { "Gator usage and context" }
	if panel.state ~= "ready" then
		table.insert(lines, "State: " .. panel.state .. " · " .. panel.reason)
	else
		for index, value in ipairs(panel.events) do
			local event = value.event
			local marker = index == panel.selected and "> " or "  "
			local session = event.provider.session_id or "unavailable"
			if value.kind == "usage" then
				table.insert(
					lines,
					marker
						.. "Usage · "
						.. event.provider.name
						.. " · session: "
						.. session
						.. " · input: "
						.. tostring(value.input or "unavailable")
						.. " · output: "
						.. tostring(value.output or "unavailable")
						.. " · total: "
						.. tostring(value.total or "unavailable")
				)
			else
				table.insert(
					lines,
					marker
						.. "Context compacted · "
						.. event.provider.name
						.. " · session: "
						.. session
						.. " · "
						.. value.before
						.. " → "
						.. value.after
				)
				table.insert(lines, "  summary: " .. value.summary)
			end
			table.insert(
				lines,
				"  event: " .. event.id .. " · run: " .. event.run_id .. " · sequence: " .. event.sequence
			)
		end
	end
	table.insert(lines, "j/k navigate · q close · ? help")
	accessibility.render(panel.buffer, lines, "gator-usage")
end

local function apply(panel, next_value)
	panel.events = next_value.events
	panel.state = next_value.state
	panel.reason = next_value.reason
	panel.on_cancel = next_value.on_cancel
	panel.selected = math.min(panel.selected or 1, math.max(#panel.events, 1))
	render(panel)
end

local function bind(panel)
	accessibility.panel(panel.buffer, { next = "j", previous = "k", cancel = "q", help = "?" }, {
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
		cancel = M.cancel,
		help = function()
			vim.notify("Gator usage: j/k navigate, q close", vim.log.levels.INFO)
		end,
	})
end

function M.open(opts)
	local next_value = value(opts)
	local panel, tabpage = current()
	if panel then
		apply(panel, next_value)
		vim.api.nvim_set_current_win(panel.window)
		return panel.window
	end
	vim.cmd("botright 14new")
	local window = vim.api.nvim_get_current_win()
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype = "gator-usage"
	vim.bo[buffer].bufhidden = "wipe"
	vim.api.nvim_win_set_buf(window, buffer)
	panel = {
		window = window,
		buffer = buffer,
		events = next_value.events,
		state = next_value.state,
		reason = next_value.reason,
		on_cancel = next_value.on_cancel,
		selected = 1,
	}
	panels[tabpage] = panel
	render(panel)
	bind(panel)
	return window
end

function M.update(opts)
	local panel = current()
	if not panel then
		fail("no usage detail panel is open in this tab")
	end
	apply(panel, value(opts))
end

function M.select(index)
	local panel = current()
	if not panel then
		fail("no usage detail panel is open in this tab")
	end
	if type(index) ~= "number" or index % 1 ~= 0 or index < 1 or index > #panel.events then
		fail("selection must identify an available event")
	end
	panel.selected = index
	render(panel)
	return provider_event.from_record(provider_event.to_record(panel.events[index].event))
end

function M.close()
	local panel, tabpage = current()
	if not panel then
		return false
	end
	vim.api.nvim_win_close(panel.window, true)
	panels[tabpage] = nil
	return true
end

function M.cancel()
	local panel = current()
	if not panel then
		return false
	end
	local on_cancel = panel.on_cancel
	M.close()
	if on_cancel then
		on_cancel()
	end
	return true
end

function M.inspect()
	local panel = current()
	if not panel then
		return nil
	end
	return { state = panel.state, reason = panel.reason, events = #panel.events, selected = panel.selected }
end

return M
