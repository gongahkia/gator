local accessibility = require("gator.ui.accessibility")
local overlay = require("gator.policy.overlay")
local panel_window = require("gator.ui.window")
local M = {}
local escalations = {}
local modes = { read_only = 0, plan = 1, default = 2 }

local function fail(message)
	error("Gator permission escalation: " .. message, 3)
end

local function current()
	local tabpage = vim.api.nvim_get_current_tabpage()
	local escalation = escalations[tabpage]
	if escalation and vim.api.nvim_win_is_valid(escalation.window) then
		return escalation, tabpage
	end
	escalations[tabpage] = nil
	return nil, tabpage
end

local function provider(value)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail("provider must be a lowercase identifier")
	end
	return value
end

local function mode(value, name)
	if type(value) ~= "string" or modes[value] == nil then
		fail(name .. " must be read_only, plan, or default")
	end
	return value
end

local function changes(baseline, requested)
	local result = {}
	for key, value in pairs(requested.rules) do
		if key:match("_allowed$") then
			if type(value) ~= "boolean" then
				fail("requested " .. key .. " must be boolean")
			end
			if baseline.rules[key] == false and value then
				table.insert(result, { key = key, before = false, after = true })
			end
		elseif key == "mode" then
			value = mode(value, "requested mode")
			local before = baseline.rules.mode or "default"
			before = mode(before, "baseline mode")
			if modes[value] > modes[before] then
				table.insert(result, { key = key, before = before, after = value })
			end
		elseif not vim.deep_equal(value, baseline.rules[key]) then
			fail("cannot assess requested policy rule: " .. key)
		end
	end
	return result
end

local function render(escalation)
	local lines = {
		"Gator permission escalation",
		"Provider: " .. escalation.provider,
		"Baseline: " .. escalation.baseline.scope .. " · " .. escalation.baseline.provenance.source,
		"Requested: " .. escalation.requested.scope .. " · " .. escalation.requested.provenance.source,
		"Acknowledgement is required before Gator requests this broader provider mode.",
	}
	for _, change in ipairs(escalation.changes) do
		table.insert(lines, "  " .. change.key .. ": " .. tostring(change.before) .. " → " .. tostring(change.after))
	end
	table.insert(lines, "<CR> acknowledge · q cancel · ? help")
	accessibility.render(escalation.buffer, lines, "gator-escalation")
end

function M.request(opts)
	if type(opts) ~= "table" then
		fail("request requires options")
	end
	for key in pairs(opts) do
		if
			key ~= "provider"
			and key ~= "baseline"
			and key ~= "requested"
			and key ~= "on_acknowledge"
			and key ~= "on_cancel"
		then
			fail("request contains unsupported field: " .. tostring(key))
		end
	end
	if not overlay.is(opts.baseline) or not overlay.is(opts.requested) then
		fail("request requires baseline and requested policy overlays")
	end
	if type(opts.on_acknowledge) ~= "function" then
		fail("request requires an acknowledgement callback")
	end
	if opts.on_cancel ~= nil and type(opts.on_cancel) ~= "function" then
		fail("on_cancel must be a function")
	end
	local value = changes(opts.baseline, opts.requested)
	if #value == 0 then
		return { required = false, changes = {} }
	end
	if current() then
		fail("an escalation acknowledgement is already open in this tab")
	end
	local opened = panel_window.open("botright " .. math.max(8, #value + 6) .. "new")
	local window = opened.window
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].bufhidden = "wipe"
	vim.api.nvim_win_set_buf(window, buffer)
	local escalation = {
		window = window,
		buffer = buffer,
		provider = provider(opts.provider),
		baseline = overlay.from_record(overlay.to_record(opts.baseline)),
		requested = overlay.from_record(overlay.to_record(opts.requested)),
		changes = value,
		on_acknowledge = opts.on_acknowledge,
		on_cancel = opts.on_cancel,
		previous = opened.previous,
	}
	local _, tabpage = current()
	escalations[tabpage] = escalation
	render(escalation)
	accessibility.panel(buffer, { confirm = "<CR>", cancel = "q", help = "?" }, {
		confirm = M.acknowledge,
		cancel = M.cancel,
		help = function()
			vim.notify("Gator escalation: <CR> acknowledge, q cancel", vim.log.levels.INFO)
		end,
	})
	return { required = true, changes = vim.deepcopy(value), window = window }
end

function M.acknowledge()
	local escalation, tabpage = current()
	if not escalation then
		fail("no escalation acknowledgement is open in this tab")
	end
	escalations[tabpage] = nil
	panel_window.close(escalation.window, escalation.previous)
	return escalation.on_acknowledge({
		provider = escalation.provider,
		baseline = overlay.to_record(escalation.baseline),
		requested = overlay.to_record(escalation.requested),
		changes = vim.deepcopy(escalation.changes),
	})
end

function M.cancel()
	local escalation, tabpage = current()
	if not escalation then
		return false
	end
	escalations[tabpage] = nil
	panel_window.close(escalation.window, escalation.previous)
	if escalation.on_cancel then
		escalation.on_cancel()
	end
	return true
end

return M
