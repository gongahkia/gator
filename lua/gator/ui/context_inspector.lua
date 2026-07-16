local pack = require("gator.context.pack")
local M = {}
local inspectors = {}

local function fail(message)
	error("Gator context inspector: " .. message, 3)
end

local function current()
	local tabpage = vim.api.nvim_get_current_tabpage()
	local inspector = inspectors[tabpage]
	if inspector and vim.api.nvim_win_is_valid(inspector.window) then
		return inspector, tabpage
	end
	inspectors[tabpage] = nil
	return nil, tabpage
end

local function estimate(entry)
	if entry.token_estimate.status == "estimated" then
		return tostring(entry.token_estimate.tokens) .. " tokens"
	end
	return "unavailable: " .. entry.token_estimate.reason
end

local function transfer(entry)
	if entry.transfer.eligible then
		return "eligible"
	end
	return "ineligible: " .. entry.transfer.reason
end

local function policy(entry)
	if entry.policy_decision then
		return entry.policy_decision
	end
	if entry.transfer.eligible then
		return "allowed by transfer eligibility"
	end
	return "blocked: " .. entry.transfer.reason
end

local function render(inspector)
	local lines = { "Gator context · " .. inspector.pack.task_id }
	for _, entry in ipairs(inspector.pack.entries) do
		local included = inspector.included[entry.id] and "included" or "excluded"
		table.insert(lines, "[" .. included .. "] " .. entry.id .. " · " .. entry.kind)
		table.insert(lines, "  ref: " .. entry.ref)
		table.insert(lines, "  source path: " .. entry.provenance.ref)
		table.insert(lines, "  revision: " .. (entry.revision or "unavailable"))
		table.insert(lines, "  retrieval source: " .. (entry.retrieval_source or entry.provenance.source))
		table.insert(lines, "  trust: " .. entry.trust .. " · tokens: " .. estimate(entry))
		table.insert(lines, "  policy: " .. policy(entry))
		table.insert(lines, "  transfer: " .. transfer(entry))
		if entry.pinned then
			table.insert(lines, "  pinned")
		end
		if entry.annotation then
			table.insert(lines, "  note: " .. entry.annotation)
		end
	end
	if #inspector.pack.entries == 0 then
		table.insert(lines, "No context entries")
	end
	vim.api.nvim_buf_set_lines(inspector.buffer, 0, -1, false, lines)
end

local function find(inspector, entry_id)
	for index, entry in ipairs(inspector.pack.entries) do
		if entry.id == entry_id then
			return index, entry
		end
	end
	fail("entry is not present in the context pack: " .. tostring(entry_id))
end

function M.open(opts)
	if type(opts) ~= "table" or not pack.is(opts.pack) or type(opts.on_confirm) ~= "function" then
		fail("open requires a context pack and on_confirm callback")
	end
	local included = {}
	for _, entry in ipairs(opts.pack.entries) do
		included[entry.id] = false
	end
	local inspector, tabpage = current()
	if inspector then
		inspector.pack = pack.from_record(pack.to_record(opts.pack))
		inspector.included = included
		inspector.on_confirm = opts.on_confirm
		render(inspector)
		vim.api.nvim_set_current_win(inspector.window)
		return inspector.window
	end
	vim.cmd("botright 14new")
	local window = vim.api.nvim_get_current_win()
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype = "gator-context"
	vim.bo[buffer].bufhidden = "wipe"
	vim.api.nvim_win_set_buf(window, buffer)
	inspector = {
		window = window,
		buffer = buffer,
		pack = pack.from_record(pack.to_record(opts.pack)),
		included = included,
		on_confirm = opts.on_confirm,
	}
	inspectors[tabpage] = inspector
	render(inspector)
	return window
end

function M.toggle(entry_id)
	local inspector = current()
	if not inspector then
		fail("no context inspector is open in this tab")
	end
	local _, entry = find(inspector, entry_id)
	if not entry.transfer.eligible then
		fail("entry is not transfer-eligible: " .. entry_id)
	end
	inspector.included[entry_id] = not inspector.included[entry_id]
	render(inspector)
	return inspector.included[entry_id]
end

function M.add(entry)
	local inspector = current()
	if not inspector then
		fail("no context inspector is open in this tab")
	end
	entry = pack.entry(entry)
	for _, existing in ipairs(inspector.pack.entries) do
		if existing.id == entry.id then
			fail("entry is already present: " .. entry.id)
		end
	end
	table.insert(inspector.pack.entries, entry)
	inspector.included[entry.id] = false
	render(inspector)
end

function M.remove(entry_id)
	local inspector = current()
	if not inspector then
		fail("no context inspector is open in this tab")
	end
	local index, entry = find(inspector, entry_id)
	table.remove(inspector.pack.entries, index)
	inspector.included[entry.id] = nil
	render(inspector)
end

function M.move(entry_id, index)
	local inspector = current()
	if not inspector then
		fail("no context inspector is open in this tab")
	end
	if type(index) ~= "number" or index % 1 ~= 0 or index < 1 or index > #inspector.pack.entries then
		fail("destination index must be within the context pack")
	end
	local current_index = find(inspector, entry_id)
	local entry = table.remove(inspector.pack.entries, current_index)
	table.insert(inspector.pack.entries, index, entry)
	render(inspector)
end

function M.annotate(entry_id, value)
	local inspector = current()
	if not inspector then
		fail("no context inspector is open in this tab")
	end
	if value ~= nil and (type(value) ~= "string" or value == "") then
		fail("annotation must be a non-empty string or nil")
	end
	local _, entry = find(inspector, entry_id)
	entry.annotation = value
	render(inspector)
end

function M.pin(entry_id, value)
	local inspector = current()
	if not inspector then
		fail("no context inspector is open in this tab")
	end
	if type(value) ~= "boolean" then
		fail("pinned value must be a boolean")
	end
	local _, entry = find(inspector, entry_id)
	entry.pinned = value
	render(inspector)
end

function M.confirm()
	local inspector = current()
	if not inspector then
		fail("no context inspector is open in this tab")
	end
	local entries = {}
	for _, pinned in ipairs({ true, false }) do
		for _, entry in ipairs(inspector.pack.entries) do
			if inspector.included[entry.id] and entry.pinned == pinned then
				table.insert(entries, entry)
			end
		end
	end
	local selected = pack.new({ id = inspector.pack.id, task_id = inspector.pack.task_id, entries = entries })
	inspector.on_confirm(selected)
	return selected
end

function M.close()
	local inspector, tabpage = current()
	if not inspector then
		return false
	end
	vim.api.nvim_win_close(inspector.window, true)
	inspectors[tabpage] = nil
	return true
end

return M
