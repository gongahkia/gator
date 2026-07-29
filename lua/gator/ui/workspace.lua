local accessibility = require("gator.ui.accessibility")
local attachments = require("gator.context.images")
local panel_window = require("gator.ui.window")
local redact = require("gator.policy.redact")
local run_state = require("gator.ui.run_state")

local M = {}
local panels_by_tab, panels_by_run = {}, {}
local settings = {
	position = "right",
	width = 40,
	height = 18,
	rotation = { "right", "bottom", "left" },
	panels = { context = true, activity = true, approvals = true },
	approval_scope = "all",
	images = { enabled = true },
}

local function fail(message)
	error("Gator workspace: " .. redact.text(tostring(message)), 3)
end

local function tabpage(value)
	return value or vim.api.nvim_get_current_tabpage()
end

local function valid(panel)
	return panel and vim.api.nvim_buf_is_valid(panel.buffer)
end

local function elapsed(panel)
	if panel.state ~= "running" or type(panel.turn_started_at) ~= "number" then
		return nil
	end
	local seconds = math.max(0, os.time() - panel.turn_started_at)
	return string.format("%d:%02d", math.floor(seconds / 60), seconds % 60)
end

local function markdown_line(line)
	line = line:gsub("^## user%s*$", "You")
	line = line:gsub("^## assistant%s*$", "Gator agent")
	line = line:gsub("%[([^%]]+)%]%b()", "%1")
	line = line:gsub("`([^`]+)`", "%1")
	return line
end

local function transcript(history)
	local lines = {}
	for _, value in ipairs(history or {}) do
		for _, line in ipairs(vim.split(redact.text(value), "\n", { plain = true, trimempty = false })) do
			table.insert(lines, markdown_line(line))
		end
	end
	return lines
end

local function action_kind(value)
	local text = (value.kind or value.action or ""):lower()
	if text:find("file", 1, true) or text:find("write", 1, true) or text:find("edit", 1, true) then
		return "edit"
	end
	if text:find("command", 1, true) or text:find("exec", 1, true) then
		return "command"
	end
	return "escalation"
end

local function details(value)
	if type(value) == "string" then
		return redact.text(value)
	end
	if type(value) == "table" then
		return redact.text(vim.json.encode(value))
	end
	return "no additional detail"
end

local function open_window(buffer, position, previous)
	if previous and vim.api.nvim_win_is_valid(previous) then
		vim.api.nvim_set_current_win(previous)
	end
	if position == "left" then
		vim.cmd("topleft vsplit")
	elseif position == "bottom" then
		vim.cmd("botright " .. settings.height .. "new")
	else
		vim.cmd("botright vsplit")
	end
	local window = vim.api.nvim_get_current_win()
	vim.api.nvim_win_set_buf(window, buffer)
	if position ~= "bottom" then
		pcall(vim.api.nvim_win_set_width, window, settings.width)
	end
	return window
end

local function render(panel)
	if not valid(panel) then
		return false
	end
	local label = panel.run_id and "active run" or "workspace"
	local header = "Gator workspace · " .. panel.provider .. " · " .. label .. " · " .. run_state.summary(panel.state)
	if panel.state == "running" then
		header = header .. " · " .. (panel.phase or "working") .. " · " .. (elapsed(panel) or "0:00")
	end
	local lines = { header, "Status: " .. run_state.detail(panel.state), "" }
	if settings.panels.context then
		table.insert(lines, "## Context")
		local context = panel.context or {}
		for _, name in ipairs({ "files", "selections", "diagnostics", "images" }) do
			local values = context[name] or {}
			if #values > 0 then
				table.insert(lines, "- " .. name .. ": " .. table.concat(values, ", "))
			end
		end
		if #lines > 0 and lines[#lines] == "## Context" then
			table.insert(lines, "- no additional context")
		end
		table.insert(lines, "")
	end
	if settings.panels.activity then
		table.insert(lines, "## Activity")
		local history = transcript(panel.history)
		if #history == 0 then
			table.insert(lines, panel.state == "running" and "Working" or "No provider output")
		else
			vim.list_extend(lines, history)
		end
		table.insert(lines, "")
	end
	if settings.panels.approvals then
		table.insert(lines, "## Approvals · " .. #panel.approvals .. " pending")
		if #panel.approvals == 0 then
			table.insert(lines, "- none")
		else
			for index, value in ipairs(panel.approvals) do
				local suffix = value.diff and " · diff ready" or (value.kind == "edit" and " · diff unavailable" or "")
				table.insert(lines, string.format("%d. %s%s", index, value.action, suffix))
				table.insert(lines, "   " .. details(value.details))
			end
		end
		table.insert(lines, "")
	end
	table.insert(lines, "i prompt · p image · 1-9 decide approval · d remove context · o rotate · c cancel · q detach · r runs")
	accessibility.render(panel.buffer, lines, "gator-workspace")
	return true
end

local function move(panel, target, force)
	target = tabpage(target)
	if not force and panel.tabpage == target and panel.window and vim.api.nvim_win_is_valid(panel.window) then
		vim.api.nvim_set_current_win(panel.window)
		return panel.window
	end
	if panel.tabpage then
		panels_by_tab[panel.tabpage] = nil
	end
	if panel.window and vim.api.nvim_win_is_valid(panel.window) then
		vim.bo[panel.buffer].bufhidden = "hide"
		vim.api.nvim_win_close(panel.window, true)
	end
	vim.api.nvim_set_current_tabpage(target)
	local previous = vim.api.nvim_get_current_win()
	panel.window = open_window(panel.buffer, panel.position, previous)
	vim.bo[panel.buffer].bufhidden = "hide"
	panel.tabpage = target
	panels_by_tab[target] = panel
	return panel.window
end

local function resolve(panel, index, decision)
	local approval = panel.approvals[index]
	if not approval then
		return false
	end
	table.remove(panel.approvals, index)
	render(panel)
	local ok, err = pcall(approval.on_decide, decision)
	if not ok then
		panel.notice = redact.text(tostring(err))
		render(panel)
		return false
	end
	return true
end

local function bind(panel)
	local buffer = panel.buffer
	vim.keymap.set("n", "i", function()
		panel.on_input()
	end, { buffer = buffer, silent = true, desc = "Gator workspace prompt" })
	vim.keymap.set("n", "c", function()
		panel.on_cancel()
	end, { buffer = buffer, silent = true, desc = "Gator workspace cancel" })
	vim.keymap.set("n", "q", function()
		M.detach(panel.run_id)
	end, { buffer = buffer, silent = true, desc = "Gator workspace detach" })
	vim.keymap.set("n", "r", function()
		panel.on_runs()
	end, { buffer = buffer, silent = true, desc = "Gator workspace runs" })
	vim.keymap.set("n", "o", function()
		M.rotate(panel.run_id)
	end, { buffer = buffer, silent = true, desc = "Gator workspace rotate" })
	vim.keymap.set("n", "d", function()
		for _, name in ipairs({ "images", "diagnostics", "selections", "files" }) do
			local values = panel.context[name]
			if values and #values > 0 then
				table.remove(values)
				break
			end
		end
		render(panel)
	end, { buffer = buffer, silent = true, desc = "Gator workspace remove context" })
	vim.keymap.set("n", "p", function()
		local image, reason = attachments.capture({ run_id = panel.run_id, enabled = settings.images.enabled })
		if image then
			table.insert(panel.context.images, vim.fn.fnamemodify(image.path, ":t"))
			panel.attachments[#panel.attachments + 1] = image
		else
			panel.notice = reason
		end
		render(panel)
	end, { buffer = buffer, silent = true, desc = "Gator workspace paste image" })
	for index = 1, 9 do
		vim.keymap.set("n", tostring(index), function()
			resolve(panel, index, "approved")
		end, { buffer = buffer, silent = true, desc = "Gator workspace approve request" })
	end
end

function M.configure(opts)
	if type(opts) ~= "table" then
		fail("settings must be an object")
	end
	settings = vim.deepcopy(opts)
	settings.images = settings.images or { enabled = true }
	return vim.deepcopy(settings)
end

function M.open(opts)
	if type(opts) ~= "table" or type(opts.run_id) ~= "string" or type(opts.provider) ~= "string" then
		fail("open requires run_id and provider")
	end
	for _, name in ipairs({ "on_input", "on_cancel", "on_detach", "on_runs" }) do
		if type(opts[name]) ~= "function" then
			fail("open requires " .. name .. " callback")
		end
	end
	local panel = panels_by_run[opts.run_id]
	if not panel then
		local buffer = vim.api.nvim_create_buf(false, true)
		vim.bo[buffer].filetype, vim.bo[buffer].bufhidden = "gator-workspace", "hide"
		panel = {
			buffer = buffer,
			run_id = opts.run_id,
			approvals = {},
			attachments = {},
			position = settings.position,
		}
		panels_by_run[opts.run_id] = panel
		bind(panel)
	end
	for name, value in pairs(opts) do
		if name ~= "history" and name ~= "context" and name ~= "attachments" then
			panel[name] = value
		end
	end
	panel.history = vim.deepcopy(opts.history or panel.history or {})
	panel.context = vim.deepcopy(opts.context or panel.context or { files = {}, selections = {}, diagnostics = {}, images = {} })
	for _, name in ipairs({ "files", "selections", "diagnostics", "images" }) do
		panel.context[name] = panel.context[name] or {}
	end
	move(panel, tabpage(opts.tabpage))
	render(panel)
	return panel.window
end

function M.update(opts)
	if type(opts) ~= "table" or type(opts.run_id) ~= "string" then
		fail("update requires run id")
	end
	local panel = panels_by_run[opts.run_id]
	if not panel then
		return false
	end
	for name, value in pairs(opts) do
		if name == "text" and type(value) == "string" and value ~= "" then
			if opts.append and type(panel.history[#panel.history]) == "string" then
				panel.history[#panel.history] = panel.history[#panel.history] .. value
			else
				table.insert(panel.history, "## " .. (opts.role == "user" and "user" or "assistant") .. "\n" .. value)
			end
		elseif name ~= "text" and name ~= "append" and name ~= "role" then
			panel[name] = value
		end
	end
	render(panel)
	return true
end

function M.request_approval(run_id, value)
	local panel = panels_by_run[run_id]
	if not panel or type(value) ~= "table" or type(value.on_decide) ~= "function" then
		return false
	end
	value = vim.deepcopy(value)
	value.kind = action_kind(value)
	table.insert(panel.approvals, value)
	render(panel)
	return true
end

function M.add_context(run_id, kind, artifacts)
	local panel = panels_by_run[run_id]
	if not panel or type(kind) ~= "string" or type(artifacts) ~= "table" then
		return false
	end
	local target = kind == "selection" and "selections" or (kind == "diagnostic" and "diagnostics" or "files")
	for _, artifact in ipairs(artifacts) do
		local label = artifact.path or artifact.bundle_id or artifact.kind
		if type(label) == "string" and label ~= "" then
			table.insert(panel.context[target], redact.text(label))
		end
	end
	render(panel)
	return true
end

function M.detach(run_id)
	local panel = panels_by_run[run_id]
	if not panel then
		return false
	end
	if panel.window and vim.api.nvim_win_is_valid(panel.window) then
		vim.api.nvim_win_close(panel.window, true)
	end
	panels_by_tab[panel.tabpage] = nil
	panel.window, panel.tabpage = nil, nil
	panel.on_detach()
	return true
end

function M.rotate(run_id)
	local panel = panels_by_run[run_id]
	if not panel then
		return false
	end
	local index = 1
	for current, value in ipairs(settings.rotation) do
		if value == panel.position then
			index = current % #settings.rotation + 1
			break
		end
	end
	panel.position = settings.rotation[index]
	move(panel, panel.tabpage, true)
	render(panel)
	return true
end

function M.close()
	local panel = panels_by_tab[tabpage()]
	if not panel then
		return false
	end
	return M.detach(panel.run_id)
end

function M.inspect(run_id)
	local panel = panels_by_run[run_id]
	if not panel then
		return nil
	end
	return {
		run_id = panel.run_id,
		tabpage = panel.tabpage,
		position = panel.position,
		approvals = #panel.approvals,
		attachments = vim.deepcopy(panel.attachments),
	}
end

return M
