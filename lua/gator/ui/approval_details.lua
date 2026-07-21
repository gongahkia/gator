local provider_event = require("gator.core.provider_event")
local redact = require("gator.policy.redact")
local accessibility = require("gator.ui.accessibility")
local panel_window = require("gator.ui.window")
local M = {}
local panels = {}
local states = { ready = true, unavailable = true, failed = true }
local decisions = { approved = true, denied = true, cancelled = true }

local function fail(message)
	error("Gator approval details: " .. redact.text(tostring(message)), 3)
end

local function text(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty string")
	end
	return redact.text(value)
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

local function request(value)
	if not provider_event.is(value) or value.type ~= "permission.request" then
		fail("request must be a normalized permission.request event")
	end
	if type(value.payload) ~= "table" then
		fail("request payload must be a table")
	end
	local request_id = text(value.payload.request_id, "request id")
	local action = text(value.payload.action, "request action")
	if
		value.payload.details ~= nil
		and type(value.payload.details) ~= "table"
		and type(value.payload.details) ~= "string"
	then
		fail("request details must be text or a table")
	end
	return {
		id = value.id,
		run_id = value.run_id,
		provider = value.provider.name,
		session_id = value.provider.session_id,
		request_id = request_id,
		action = action,
		details = redact.value(vim.deepcopy(value.payload.details or {})),
	}
end

local function value(opts)
	if type(opts) ~= "table" then
		fail("open requires options")
	end
	for key in pairs(opts) do
		if key ~= "request" and key ~= "state" and key ~= "reason" and key ~= "on_decide" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	local state = opts.state or "ready"
	if not states[state] then
		fail("state must be ready, unavailable, or failed")
	end
	if state == "ready" then
		if type(opts.on_decide) ~= "function" then
			fail("ready state requires an on_decide callback")
		end
		return { state = state, request = request(opts.request), on_decide = opts.on_decide }
	end
	if opts.request ~= nil or opts.on_decide ~= nil then
		fail("unavailable or failed state must not contain a request callback")
	end
	return {
		state = state,
		reason = text(
			opts.reason or (state == "unavailable" and "approval request is unavailable" or "approval request failed"),
			"reason"
		),
	}
end

local function render(panel)
	local lines = { "Gator approval request" }
	if panel.state ~= "ready" then
		table.insert(lines, "State: " .. panel.state .. " · " .. panel.reason)
		table.insert(lines, "q close · ? help")
		accessibility.render(panel.buffer, lines, "gator-approval")
		return
	end
	local request = panel.request
	table.insert(lines, "Provider: " .. request.provider .. " · session: " .. (request.session_id or "unavailable"))
	table.insert(lines, "Request: " .. request.request_id .. " · action: " .. request.action)
	table.insert(lines, "Event: " .. request.id .. " · run: " .. request.run_id)
	local details = type(request.details) == "string" and request.details or vim.json.encode(request.details)
	for index, line in ipairs(vim.split(details, "\n", { plain = true, trimempty = false })) do
		table.insert(lines, index == 1 and "Details: " .. line or "  " .. line)
	end
	table.insert(lines, "<CR> approve · d deny · q cancel · ? help")
	accessibility.render(panel.buffer, lines, "gator-approval")
end

local function set_failure(panel, detail)
	panel.state = "failed"
	panel.reason = redact.text(detail)
	panel.request = nil
	panel.on_decide = nil
	render(panel)
end

local function apply(panel, next_value)
	panel.state = next_value.state
	panel.request = next_value.request
	panel.reason = next_value.reason
	panel.on_decide = next_value.on_decide
	render(panel)
end

local function bind(panel)
	accessibility.panel(panel.buffer, { accept = "<CR>", reject = "d", cancel = "q", help = "?" }, {
		accept = M.approve,
		reject = M.deny,
		cancel = M.cancel,
		help = function()
			vim.notify("Gator approval: <CR> approve, d deny, q cancel", vim.log.levels.INFO)
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
	local opened = panel_window.open("botright 12new")
	local window = opened.window
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype = "gator-approval"
	vim.bo[buffer].bufhidden = "wipe"
	vim.api.nvim_win_set_buf(window, buffer)
	panel = {
		window = window,
		buffer = buffer,
		state = next_value.state,
		request = next_value.request,
		reason = next_value.reason,
		on_decide = next_value.on_decide,
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
		fail("no approval detail panel is open in this tab")
	end
	apply(panel, value(opts))
end

function M.decide(decision)
	if not decisions[decision] then
		fail("decision must be approved, denied, or cancelled")
	end
	local panel = current()
	if not panel then
		fail("no approval detail panel is open in this tab")
	end
	if panel.state ~= "ready" then
		return false
	end
	local request = panel.request
	local record = {
		provider = request.provider,
		session_id = request.session_id,
		request_id = request.request_id,
		action = request.action,
		decision = decision,
	}
	local ok, accepted, detail = pcall(panel.on_decide, vim.deepcopy(record))
	if not ok then
		set_failure(panel, "decision callback failed: " .. tostring(accepted))
		return false
	end
	if accepted == false then
		set_failure(panel, type(detail) == "string" and detail or "provider rejected the decision")
		return false
	end
	M.close()
	return true
end

function M.approve()
	return M.decide("approved")
end

function M.deny()
	return M.decide("denied")
end

function M.cancel()
	local panel = current()
	if not panel then
		return false
	end
	if panel.state == "ready" then
		return M.decide("cancelled")
	end
	return M.close()
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

function M.inspect()
	local panel = current()
	if not panel then
		return nil
	end
	return {
		state = panel.state,
		reason = panel.reason,
		request_id = panel.request and panel.request.request_id or nil,
	}
end

return M
