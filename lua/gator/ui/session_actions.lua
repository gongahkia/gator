local capabilities = require("gator.adapters.capabilities")
local deep_link = require("gator.core.deep_link")
local session = require("gator.core.session")
local redact = require("gator.policy.redact")
local accessibility = require("gator.ui.accessibility")
local M = {}
local panels = {}

local function fail(message)
	error("Gator session actions: " .. redact.text(tostring(message)), 3)
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

local function value(opts, cancelled)
	if type(opts) ~= "table" then
		fail("open requires options")
	end
	for key in pairs(opts) do
		if
			key ~= "session"
			and key ~= "capabilities"
			and key ~= "native_resolve"
			and key ~= "on_open"
			and key ~= "on_reference"
			and key ~= "on_cancel"
		then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	if not session.is(opts.session) or not capabilities.is(opts.capabilities) then
		fail("open requires a provider-owned session and capability contract")
	end
	if opts.capabilities.provider ~= opts.session.provider then
		fail("capability provider must match the session provider")
	end
	for _, name in ipairs({ "native_resolve", "on_open", "on_reference" }) do
		if type(opts[name]) ~= "function" then
			fail(name .. " must be a function")
		end
	end
	if opts.on_cancel ~= nil and type(opts.on_cancel) ~= "function" then
		fail("on_cancel must be a function")
	end
	local link = deep_link.resolve({
		session = opts.session,
		capabilities = opts.capabilities,
		native_resolve = opts.native_resolve,
		cancelled = cancelled,
	})
	return {
		reference = session.reference(opts.session),
		link = link,
		on_open = opts.on_open,
		on_reference = opts.on_reference,
		on_cancel = opts.on_cancel,
	}
end

local function actions(panel)
	local result = { { id = "reference", label = "Use provider-native session reference" } }
	if panel.link.status == "available" then
		table.insert(result, 1, { id = "open", label = "Open provider-native link" })
	end
	return result
end

local function render(panel)
	local reference, link = panel.reference, panel.link
	local lines = {
		"Gator provider-native session",
		"Provider: " .. reference.provider .. " · session: " .. reference.id .. " · owner: " .. reference.owner,
		"Deep link: "
			.. link.status
			.. (link.uri and " · " .. link.uri or link.reason and " · " .. link.reason or ""),
	}
	for index, action in ipairs(actions(panel)) do
		table.insert(lines, (index == panel.selected and "> " or "  ") .. action.label)
	end
	table.insert(lines, "j/k navigate · <CR> confirm · q close · ? help")
	accessibility.render(panel.buffer, lines, "gator-session-actions")
end

local function set_failure(panel, reason)
	panel.link = { status = "failed", reason = redact.text(reason) }
	render(panel)
end

local function apply(panel, next_value)
	panel.reference = next_value.reference
	panel.link = next_value.link
	panel.on_open = next_value.on_open
	panel.on_reference = next_value.on_reference
	panel.on_cancel = next_value.on_cancel
	panel.selected = math.min(panel.selected or 1, #actions(panel))
	render(panel)
end

local function bind(panel)
	accessibility.panel(panel.buffer, { next = "j", previous = "k", confirm = "<CR>", cancel = "q", help = "?" }, {
		next = function()
			panel.selected = panel.selected % #actions(panel) + 1
			render(panel)
		end,
		previous = function()
			panel.selected = (panel.selected - 2) % #actions(panel) + 1
			render(panel)
		end,
		confirm = M.activate,
		cancel = M.cancel,
		help = function()
			vim.notify("Gator session actions: j/k navigate, <CR> confirm, q close", vim.log.levels.INFO)
		end,
	})
end

function M.open(opts)
	local cancelled = false
	local next_value = value(opts, function()
		return cancelled
	end)
	local panel, tabpage = current()
	if panel then
		apply(panel, next_value)
		vim.api.nvim_set_current_win(panel.window)
		return panel.window
	end
	vim.cmd("botright 10new")
	local window = vim.api.nvim_get_current_win()
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype = "gator-session-actions"
	vim.bo[buffer].bufhidden = "wipe"
	vim.api.nvim_win_set_buf(window, buffer)
	panel = {
		window = window,
		buffer = buffer,
		reference = next_value.reference,
		link = next_value.link,
		on_open = next_value.on_open,
		on_reference = next_value.on_reference,
		on_cancel = next_value.on_cancel,
		selected = 1,
		cancelled = function()
			cancelled = true
		end,
	}
	panels[tabpage] = panel
	render(panel)
	bind(panel)
	return window
end

function M.activate()
	local panel = current()
	if not panel then
		fail("no session action panel is open in this tab")
	end
	local action = actions(panel)[panel.selected]
	local callback, args
	if action.id == "open" then
		callback = panel.on_open
		args = { panel.link.uri, vim.deepcopy(panel.reference) }
	else
		callback = panel.on_reference
		args = { vim.deepcopy(panel.reference) }
	end
	local ok, accepted, reason = pcall(callback, unpack(args))
	if not ok then
		set_failure(panel, "session action failed: " .. tostring(accepted))
		return false
	end
	if accepted == false then
		set_failure(panel, type(reason) == "string" and reason or "provider rejected the session action")
		return false
	end
	return true
end

function M.select(index)
	local panel = current()
	if not panel then
		fail("no session action panel is open in this tab")
	end
	if type(index) ~= "number" or index % 1 ~= 0 or index < 1 or index > #actions(panel) then
		fail("selection must identify an available session action")
	end
	panel.selected = index
	render(panel)
	return actions(panel)[index].id
end

function M.close()
	local panel, tabpage = current()
	if not panel then
		return false
	end
	panel.cancelled()
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
	return {
		link_status = panel.link.status,
		link_reason = panel.link.reason,
		actions = #actions(panel),
		selected = panel.selected,
	}
end

return M
