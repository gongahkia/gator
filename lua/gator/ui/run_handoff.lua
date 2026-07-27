local accessibility = require("gator.ui.accessibility")
local panel_window = require("gator.ui.window")
local redact = require("gator.policy.redact")

local M = {}
local panels = {}

local function fail(message)
	error("Gator run handoff: " .. redact.text(tostring(message)), 3)
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

local function render(panel)
	local lines = {
		"Gator handoff review",
		"Source: " .. panel.source.provider .. " · target: " .. panel.target .. " · profile: " .. panel.profile,
		"Transcript: " .. panel.source.transcript,
		"Target transport: " .. panel.preflight.transport .. " · workspace: " .. panel.preflight.workspace,
		"Context: "
			.. panel.preflight.context_bytes
			.. " bytes · snapshots "
			.. panel.preflight.included
			.. " included / "
			.. panel.preflight.omitted
			.. " omitted",
		"Snapshot application: "
			.. (panel.apply_snapshot and "choose apply or retain before launch" or "retained only"),
		"",
	}
	if #panel.conflicts > 0 then
		table.insert(lines, "Target conflicts: " .. #panel.conflicts .. " · choose apply/skip for each on launch")
		for _, conflict in ipairs(panel.conflicts) do
			table.insert(lines, "- " .. redact.text(conflict.path) .. " · " .. redact.text(conflict.reason))
		end
		table.insert(lines, "")
	end
	if panel.preview and panel.preview ~= "" then
		vim.list_extend(lines, { "## Target-worktree diff", "", "```diff", panel.preview, "```", "" })
	end
	vim.list_extend(lines, vim.split(panel.body, "\n", { plain = true, trimempty = false }))
	table.insert(lines, "")
	table.insert(lines, "<CR> launch new session · e edit note/bundle · q cancel · ? help")
	accessibility.render(panel.buffer, lines, "gator-handoff")
end

local function confirm(panel)
	local decisions = {}
	local function complete(apply_snapshot)
		panel.confirmed = true
		panel.on_confirm(panel.body, decisions, apply_snapshot)
		M.close()
	end
	local function next_conflict(index)
		local conflict = panel.conflicts[index]
		if not conflict then
			complete(true)
			return
		end
		vim.ui.select({ "Apply source snapshot", "Skip source snapshot", "Cancel" }, {
			prompt = "Gator handoff conflict · " .. redact.text(conflict.path) .. " · " .. redact.text(
				conflict.reason
			),
		}, function(choice)
			if choice == "Apply source snapshot" then
				decisions[conflict.path] = "apply"
				next_conflict(index + 1)
			elseif choice == "Skip source snapshot" then
				decisions[conflict.path] = "skip"
				next_conflict(index + 1)
			end
		end)
	end
	if not panel.apply_snapshot then
		complete(false)
		return
	end
	vim.ui.select({ "Apply snapshots and launch", "Retain snapshots only", "Cancel" }, {
		prompt = "Gator handoff snapshot application",
	}, function(choice)
		if choice == "Apply snapshots and launch" then
			next_conflict(1)
		elseif choice == "Retain snapshots only" then
			complete(false)
		end
	end)
end

function M.open(opts)
	if
		type(opts) ~= "table"
		or type(opts.source) ~= "table"
		or type(opts.target) ~= "string"
		or type(opts.body) ~= "string"
		or type(opts.preflight) ~= "table"
		or type(opts.preflight.transport) ~= "string"
		or type(opts.preflight.workspace) ~= "string"
		or type(opts.preflight.context_bytes) ~= "number"
		or type(opts.preflight.included) ~= "number"
		or type(opts.preflight.omitted) ~= "number"
		or type(opts.apply_snapshot) ~= "boolean"
		or type(opts.on_confirm) ~= "function"
	then
		fail("open requires source, target, body, preflight, and confirm callback")
	end
	if opts.preview ~= nil and type(opts.preview) ~= "string" then
		fail("preview must be text")
	end
	if opts.conflicts ~= nil and (type(opts.conflicts) ~= "table" or not vim.islist(opts.conflicts)) then
		fail("conflicts must be an array")
	end
	if opts.on_cancel ~= nil and type(opts.on_cancel) ~= "function" then
		fail("cancel callback must be a function")
	end
	local panel, tabpage = current()
	if panel then
		if panel.on_cancel and not panel.confirmed then
			panel.on_cancel()
		end
		panel.source, panel.target, panel.profile, panel.body, panel.on_confirm =
			opts.source, opts.target, opts.profile, opts.body, opts.on_confirm
		panel.preview, panel.conflicts, panel.on_cancel, panel.confirmed, panel.preflight, panel.apply_snapshot =
			opts.preview,
			vim.deepcopy(opts.conflicts or {}),
			opts.on_cancel,
			false,
			vim.deepcopy(opts.preflight),
			opts.apply_snapshot
		render(panel)
		vim.api.nvim_set_current_win(panel.window)
		return panel.window
	end
	local opened = panel_window.open("botright 18new")
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype, vim.bo[buffer].bufhidden = "gator-handoff", "wipe"
	vim.api.nvim_win_set_buf(opened.window, buffer)
	panel = {
		window = opened.window,
		buffer = buffer,
		previous = opened.previous,
		source = vim.deepcopy(opts.source),
		target = opts.target,
		profile = opts.profile,
		body = redact.text(opts.body),
		on_confirm = opts.on_confirm,
		preview = redact.text(opts.preview or ""),
		conflicts = vim.deepcopy(opts.conflicts or {}),
		preflight = vim.deepcopy(opts.preflight),
		apply_snapshot = opts.apply_snapshot,
		on_cancel = opts.on_cancel,
		confirmed = false,
	}
	panels[tabpage] = panel
	render(panel)
	accessibility.panel(buffer, { confirm = "<CR>", edit = "e", cancel = "q", help = "?" }, {
		confirm = function()
			confirm(panel)
		end,
		edit = function()
			vim.ui.input({ prompt = "Gator handoff note/bundle: ", default = panel.body }, function(value)
				if type(value) == "string" and vim.trim(value) ~= "" then
					panel.body = redact.text(value)
					render(panel)
				end
			end)
		end,
		cancel = M.close,
		help = function()
			vim.notify(
				"Gator handoff: <CR> launch, e edit, q cancel; conflicts require per-file apply/skip",
				vim.log.levels.INFO
			)
		end,
	})
	return panel.window
end

function M.close()
	local panel, tabpage = current()
	if not panel then
		return false
	end
	if panel.on_cancel and not panel.confirmed then
		panel.on_cancel()
	end
	panel_window.close(panel.window, panel.previous)
	panels[tabpage] = nil
	return true
end

return M
