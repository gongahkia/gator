local provider_event = require("gator.core.provider_event")
local accessibility = require("gator.ui.accessibility")
local panel_window = require("gator.ui.window")
local M = {}
local panels = {}
local states = { ready = true, unavailable = true, failed = true }
local types =
	{ ["message.started"] = true, ["message.delta"] = true, ["message.completed"] = true, ["message.thought"] = true }

local function fail(message)
	error("Gator event details: " .. message, 3)
end

local function reason(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty string")
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

local function events(value)
	if type(value) ~= "table" or not vim.islist(value) then
		fail("events must be an array")
	end
	local result = {}
	for _, event in ipairs(value) do
		if not provider_event.is(event) then
			fail("events must contain normalized provider events")
		end
		if types[event.type] then
			table.insert(result, provider_event.from_record(provider_event.to_record(event)))
		end
	end
	table.sort(result, function(left, right)
		return left.sequence == right.sequence and left.id < right.id or left.sequence < right.sequence
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
		fail("ready state requires a message or reasoning event")
	end
	if state ~= "ready" and #result > 0 then
		fail("unavailable or failed state must not contain events")
	end
	local detail_reason = opts.reason
	if state ~= "ready" and detail_reason == nil then
		detail_reason = state == "unavailable" and "no message or reasoning events are available"
			or "event detail rendering failed"
	end
	if detail_reason ~= nil then
		detail_reason = reason(detail_reason, "reason")
	end
	return { events = result, state = state, reason = detail_reason, on_cancel = opts.on_cancel }
end

local function event_label(event)
	if event.type == "message.thought" then
		return "Reasoning summary"
	end
	return "Message " .. event.type:match("%.(.+)$")
end

local function payload(event)
	if event.type == "message.thought" and type(event.payload.summary) == "string" then
		return "summary", event.payload.summary
	end
	if type(event.payload.text) == "string" then
		return "text", event.payload.text
	end
	if type(event.payload.summary) == "string" then
		return "summary", event.payload.summary
	end
	return "details", vim.json.encode(event.payload)
end

local function render(panel)
	local lines = { "Gator message and reasoning details" }
	if panel.state ~= "ready" then
		table.insert(lines, "State: " .. panel.state .. " · " .. panel.reason)
	else
		for index, event in ipairs(panel.events) do
			local key, detail = payload(event)
			local session = event.provider.session_id or "unavailable"
			table.insert(
				lines,
				(index == panel.selected and "> " or "  ")
					.. event_label(event)
					.. " · "
					.. event.provider.name
					.. " · session: "
					.. session
			)
			table.insert(
				lines,
				"  event: " .. event.id .. " · run: " .. event.run_id .. " · sequence: " .. event.sequence
			)
			for line_index, line in ipairs(vim.split(detail, "\n", { plain = true, trimempty = false })) do
				table.insert(lines, line_index == 1 and "  " .. key .. ": " .. line or "  " .. line)
			end
		end
	end
	table.insert(lines, "j/k navigate · q close · ? help")
	accessibility.render(panel.buffer, lines, "gator-event-details")
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
			require("gator.ui.notice").show("Gator details: j/k navigate, q close", vim.log.levels.INFO)
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
	local opened = panel_window.open("botright 14new")
	local window = opened.window
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype = "gator-event-details"
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
		previous = opened.previous,
	}
	panels[tabpage] = panel
	render(panel)
	bind(panel)
	return window
end

function M.update(opts)
	local panel = current()
	if not panel then
		fail("no event detail panel is open in this tab")
	end
	apply(panel, value(opts))
end

function M.select(index)
	local panel = current()
	if not panel then
		fail("no event detail panel is open in this tab")
	end
	if type(index) ~= "number" or index % 1 ~= 0 or index < 1 or index > #panel.events then
		fail("selection must identify an available event")
	end
	panel.selected = index
	render(panel)
	return provider_event.from_record(provider_event.to_record(panel.events[index]))
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
	return {
		state = panel.state,
		reason = panel.reason,
		events = #panel.events,
		selected = panel.selected,
	}
end

return M
