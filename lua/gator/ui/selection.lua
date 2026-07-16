local pack = require("gator.context.pack")
local M = {}

local function fail(message)
	error("Gator selection: " .. message, 3)
end

local function target(value)
	if type(value) ~= "string" then
		fail("target must be task:<id> or session:<provider>:<id>")
	end
	local task_id = value:match("^task:([a-z][a-z0-9_-]*)$")
	if task_id then
		return { kind = "task", id = task_id }
	end
	local provider, session_id = value:match("^session:([^:]+):(.+)$")
	if provider and session_id ~= "" then
		return { kind = "session", provider = provider, id = session_id }
	end
	fail("target must be task:<id> or session:<provider>:<id>")
end

local function source(value)
	if type(value) ~= "table" then
		fail("source must be a table")
	end
	local buffer = value.buffer
	local first_line = value.first_line
	local last_line = value.last_line
	if type(buffer) ~= "number" or buffer < 1 or not vim.api.nvim_buf_is_valid(buffer) then
		fail("source buffer must be valid")
	end
	if type(first_line) ~= "number" or first_line % 1 ~= 0 or type(last_line) ~= "number" or last_line % 1 ~= 0 then
		fail("source line range must use integers")
	end
	local line_count = vim.api.nvim_buf_line_count(buffer)
	if first_line < 1 or last_line < first_line or last_line > line_count then
		fail("source line range must be within the buffer")
	end
	return buffer, first_line, last_line
end

local function sequence(state)
	if type(state) ~= "table" or type(state.context) ~= "table" then
		fail("capture requires initialized Gator state")
	end
	local value = state.context.selection_sequence or 0
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail("selection sequence must be a non-negative integer")
	end
	state.context.selection_sequence = value + 1
	return state.context.selection_sequence
end

local function surrounding(buffer, first_line, last_line, line_count)
	local before_first = math.max(1, first_line - 3)
	local after_last = math.min(line_count, last_line + 3)
	return {
		before = {
			first_line = before_first,
			last_line = first_line - 1,
			lines = vim.api.nvim_buf_get_lines(buffer, before_first - 1, first_line - 1, false),
		},
		after = {
			first_line = last_line + 1,
			last_line = after_last,
			lines = vim.api.nvim_buf_get_lines(buffer, last_line, after_last, false),
		},
	}
end

function M.capture(state, target_value, source_value)
	local destination = target(target_value)
	local buffer, first_line, last_line = source(source_value)
	local line_count = vim.api.nvim_buf_line_count(buffer)
	local id = "selection-" .. sequence(state)
	local name = vim.api.nvim_buf_get_name(buffer)
	if name == "" then
		name = "buffer:" .. buffer
	end
	local reference = name .. ":" .. first_line .. "-" .. last_line
	local entry = pack.entry({
		id = id,
		kind = "selection",
		ref = reference,
		provenance = { source = "buffer", ref = reference },
		trust = "manual",
		token_estimate = { status = "unavailable", reason = "selection content has not been provider-counted" },
		transfer = { eligible = true },
	})
	local record = {
		target = destination,
		entry = entry,
		lines = vim.api.nvim_buf_get_lines(buffer, first_line - 1, last_line, false),
		language = vim.bo[buffer].filetype,
		revision = vim.api.nvim_buf_get_changedtick(buffer),
		range = { first_line = first_line, last_line = last_line },
		surrounding = surrounding(buffer, first_line, last_line, line_count),
	}
	state.context.selections = state.context.selections or {}
	if type(state.context.selections) ~= "table" or not vim.islist(state.context.selections) then
		fail("selection store must be an array")
	end
	table.insert(state.context.selections, record)
	return vim.deepcopy(record)
end

return M
