local capabilities = require("gator.adapters.capabilities")
local handoff = require("gator.context.handoff")
local transfer = require("gator.context.transfer")
local redact = require("gator.policy.redact")
local accessibility = require("gator.ui.accessibility")
local panel_window = require("gator.ui.window")
local M = {}
local panels = {}
local modes = { manual = true, explicit = true, automatic = true }

local function fail(message)
	error("Gator handoff review: " .. redact.text(tostring(message)), 3)
end

local function text(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be non-empty text")
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

local function value(opts)
	if type(opts) ~= "table" then
		fail("open requires options")
	end
	for key in pairs(opts) do
		if
			key ~= "mode"
			and key ~= "source_provider"
			and key ~= "target_provider"
			and key ~= "content"
			and key ~= "target_capabilities"
			and key ~= "opt_in"
			and key ~= "on_confirm"
			and key ~= "on_cancel"
		then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	if not modes[opts.mode] then
		fail("mode must be manual, explicit, or automatic")
	end
	if not capabilities.is(opts.target_capabilities) or opts.target_capabilities.provider ~= opts.target_provider then
		fail("target capabilities must match the target provider")
	end
	if type(opts.opt_in) ~= "boolean" then
		fail("opt_in must be boolean")
	end
	if type(opts.on_confirm) ~= "function" then
		fail("on_confirm must be a function")
	end
	if opts.on_cancel ~= nil and type(opts.on_cancel) ~= "function" then
		fail("on_cancel must be a function")
	end
	local preflight = handoff.tools({ capabilities = opts.target_capabilities })
	local state = preflight.available == false and "unavailable" or "ready"
	local reason = state == "unavailable" and redact.text(preflight.reason) or nil
	if opts.mode == "automatic" and not opts.opt_in then
		state, reason = "unavailable", "automatic summary transfer requires opt-in"
	end
	return {
		mode = opts.mode,
		source_provider = text(opts.source_provider, "source provider"),
		target_provider = text(opts.target_provider, "target provider"),
		content = text(opts.content, "content"),
		preflight = preflight,
		opt_in = opts.opt_in,
		on_confirm = opts.on_confirm,
		on_cancel = opts.on_cancel,
		state = state,
		reason = reason,
	}
end

local function render(panel)
	local lines = {
		"Gator handoff review",
		"Source: " .. panel.source_provider .. " · target: " .. panel.target_provider .. " · mode: " .. panel.mode,
		"Target preflight: "
			.. (
				panel.preflight.available == false and "unavailable · " .. panel.preflight.reason
				or panel.preflight.transport
			),
	}
	if panel.state ~= "ready" then
		table.insert(lines, "State: " .. panel.state .. " · " .. panel.reason)
	else
		table.insert(lines, "Summary:")
		for _, line in ipairs(vim.split(panel.content, "\n", { plain = true, trimempty = false })) do
			table.insert(lines, "  " .. line)
		end
		table.insert(
			lines,
			panel.mode == "automatic" and "<CR> confirm · q cancel · ? help"
				or "<CR> confirm · e edit · q cancel · ? help"
		)
	end
	if panel.state ~= "ready" then
		table.insert(lines, "q close · ? help")
	end
	accessibility.render(panel.buffer, lines, "gator-handoff")
end

local function set_failure(panel, reason)
	panel.state = "failed"
	panel.reason = redact.text(reason)
	render(panel)
end

local function apply(panel, next_value)
	for key, value in pairs(next_value) do
		panel[key] = value
	end
	render(panel)
end

local function bind(panel)
	local defaults = { confirm = "<CR>", cancel = "q", help = "?" }
	local handlers = {
		confirm = M.confirm,
		cancel = M.cancel,
		help = function()
			vim.notify("Gator handoff: <CR> confirm, e edit when allowed, q cancel", vim.log.levels.INFO)
		end,
	}
	if panel.mode ~= "automatic" then
		defaults.prompt = "e"
		handlers.prompt = function()
			vim.ui.input({ prompt = "Gator handoff summary: ", default = panel.content }, function(content)
				if content ~= nil then
					M.edit(content)
				end
			end)
		end
	end
	accessibility.panel(panel.buffer, defaults, handlers)
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
	vim.bo[buffer].filetype = "gator-handoff"
	vim.bo[buffer].bufhidden = "wipe"
	vim.api.nvim_win_set_buf(window, buffer)
	panel = next_value
	panel.window, panel.buffer = window, buffer
	panel.previous = opened.previous
	panels[tabpage] = panel
	render(panel)
	bind(panel)
	return window
end

function M.edit(content)
	local panel = current()
	if not panel then
		fail("no handoff review panel is open in this tab")
	end
	if panel.state ~= "ready" or panel.mode == "automatic" then
		fail("handoff summary is unavailable for editing")
	end
	panel.content = text(content, "content")
	render(panel)
	return panel.content
end

function M.confirm()
	local panel = current()
	if not panel then
		fail("no handoff review panel is open in this tab")
	end
	if panel.state ~= "ready" then
		return false
	end
	local summary = transfer.summary({
		mode = panel.mode,
		source_provider = panel.source_provider,
		target_provider = panel.target_provider,
		content = panel.content,
		confirmed = panel.mode ~= "automatic",
		opt_in = panel.opt_in,
	})
	if not summary.available then
		panel.state, panel.reason = "unavailable", redact.text(summary.reason)
		render(panel)
		return false
	end
	local ok, accepted, reason = pcall(panel.on_confirm, vim.deepcopy(summary), vim.deepcopy(panel.preflight))
	if not ok then
		set_failure(panel, "handoff callback failed: " .. tostring(accepted))
		return false
	end
	if accepted == false then
		set_failure(panel, type(reason) == "string" and reason or "target rejected the handoff")
		return false
	end
	M.close()
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
		mode = panel.mode,
		transport = panel.preflight.transport,
		editable = panel.mode ~= "automatic" and panel.state == "ready",
	}
end

return M
