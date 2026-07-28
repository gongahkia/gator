local accessibility = require("gator.ui.accessibility")
local panel_window = require("gator.ui.window")

local M = {}
local panels = {}

local function fail(message)
	error("Gator context preflight: " .. tostring(message), 3)
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

local function artifact(value)
	if type(value) ~= "table" or type(value.kind) ~= "string" or type(value.bytes) ~= "number" then
		fail("artifact is invalid")
	end
	return value
end

local function render(panel)
	local preflight = panel.preflight
	local lines = {
		"Gator context confirmation",
		"Target: " .. preflight.provider .. " · " .. preflight.transport,
		"Payload: "
			.. preflight.bytes
			.. " bytes · ~"
			.. preflight.tokens
			.. " tokens · redactions "
			.. preflight.redactions,
		"",
		"Artifacts:",
	}
	for _, value in ipairs(preflight.artifacts) do
		local location = value.path and (" · " .. value.path) or ""
		if value.first_line then
			location = location .. " · lines " .. value.first_line .. "-" .. value.last_line
		end
		if value.count then
			location = location .. " · " .. value.count .. " entries"
		end
		table.insert(lines, "- " .. value.kind .. location .. " · " .. value.bytes .. " bytes")
	end
	table.insert(lines, "")
	table.insert(lines, "<CR> send shown context · q cancel · ? help")
	accessibility.render(panel.buffer, lines, "gator-context-preflight")
end

function M.open(opts)
	if type(opts) ~= "table" or type(opts.preflight) ~= "table" or type(opts.on_confirm) ~= "function" then
		fail("open requires preflight metadata and a confirm callback")
	end
	local preflight = vim.deepcopy(opts.preflight)
	if type(preflight.provider) ~= "string" or type(preflight.transport) ~= "string" then
		fail("preflight target is invalid")
	end
	for _, field in ipairs({ "bytes", "tokens", "redactions" }) do
		if type(preflight[field]) ~= "number" or preflight[field] < 0 then
			fail("preflight." .. field .. " must be non-negative")
		end
	end
	if type(preflight.artifacts) ~= "table" or not vim.islist(preflight.artifacts) then
		fail("preflight.artifacts must be an array")
	end
	for _, value in ipairs(preflight.artifacts) do
		artifact(value)
	end
	local panel, tabpage = current()
	if panel then
		panel.preflight, panel.on_confirm, panel.on_cancel, panel.confirmed =
			preflight, opts.on_confirm, opts.on_cancel, false
		render(panel)
		vim.api.nvim_set_current_win(panel.window)
		return panel.window
	end
	local opened = panel_window.open("botright 16new")
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype, vim.bo[buffer].bufhidden = "gator-context-preflight", "wipe"
	vim.api.nvim_win_set_buf(opened.window, buffer)
	panel = {
		window = opened.window,
		buffer = buffer,
		previous = opened.previous,
		preflight = preflight,
		on_confirm = opts.on_confirm,
		on_cancel = opts.on_cancel,
		confirmed = false,
	}
	panels[tabpage] = panel
	render(panel)
	accessibility.panel(buffer, { confirm = "<CR>", close = "q", help = "?" }, {
		confirm = function()
			panel.confirmed = true
			panel.on_confirm()
			M.close()
		end,
		close = M.close,
		help = function()
			require("gator.ui.notice").show("Gator context: <CR> send exactly this context, q cancel", vim.log.levels.INFO)
		end,
	})
	return panel.window
end

function M.close()
	local panel, tabpage = current()
	if not panel then
		return false
	end
	if not panel.confirmed and type(panel.on_cancel) == "function" then
		panel.on_cancel()
	end
	panel_window.close(panel.window, panel.previous)
	panels[tabpage] = nil
	return true
end

return M
