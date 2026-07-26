local redact = require("gator.policy.redact")

local M = {}

local function fail(message)
	error("Gator context capture: " .. redact.text(tostring(message)), 3)
end

local function text(value, name)
	if type(value) ~= "string" or vim.trim(value) == "" then
		fail(name .. " must be non-empty text")
	end
	return value
end

local function source(opts)
	if type(opts) ~= "table" then
		fail("capture options must be an object")
	end
	local buffer = opts.buffer or vim.api.nvim_get_current_buf()
	if type(buffer) ~= "number" or not vim.api.nvim_buf_is_valid(buffer) then
		fail("buffer must be valid")
	end
	local line_count = vim.api.nvim_buf_line_count(buffer)
	local first_line = opts.first_line or vim.api.nvim_win_get_cursor(0)[1]
	local last_line = opts.last_line or first_line
	if
		type(first_line) ~= "number"
		or type(last_line) ~= "number"
		or first_line < 1
		or last_line < first_line
		or last_line > line_count
	then
		fail("line range is outside the buffer")
	end
	local path = vim.api.nvim_buf_get_name(buffer)
	if path == "" then
		path = "[unnamed buffer " .. buffer .. "]"
	end
	local lines = vim.api.nvim_buf_get_lines(buffer, first_line - 1, last_line, false)
	local body = table.concat(lines, "\n")
	return {
		buffer = buffer,
		path = path,
		language = vim.bo[buffer].filetype ~= "" and vim.bo[buffer].filetype or "text",
		first_line = first_line,
		last_line = last_line,
		text = redact.text(body),
		changedtick = vim.api.nvim_buf_get_changedtick(buffer),
	}
end

function M.current(opts)
	return vim.deepcopy(source(opts or {}))
end

function M.estimate(value)
	value = text(value, "text")
	return math.max(1, math.ceil(#value / 4))
end

function M.diff(root)
	root = text(root, "project root")
	local result = vim.system({ "git", "diff", "--no-ext-diff", "--", "." }, { cwd = root, text = true }):wait()
	if result.code ~= 0 then
		return nil
	end
	local value = vim.trim(result.stdout or "")
	return value ~= "" and redact.text(value) or nil
end

local function section(lines, heading, value)
	if value and value ~= "" then
		table.insert(lines, "## " .. heading)
		table.insert(lines, value)
		table.insert(lines, "")
	end
end

function M.bundle(opts)
	if type(opts) ~= "table" then
		fail("bundle options must be an object")
	end
	local objective = text(opts.objective, "objective")
	local capture = opts.capture
	if type(capture) ~= "table" then
		fail("bundle capture is required")
	end
	local profile = opts.profile or "full"
	if profile ~= "full" and profile ~= "compact" and profile ~= "summary-first" then
		fail("bundle profile is unavailable")
	end
	local max_chars = opts.max_chars or 4096
	if type(max_chars) ~= "number" or max_chars < 1 then
		fail("bundle max_chars must be positive")
	end
	local captured = capture.text
	if profile == "compact" or profile == "summary-first" then
		captured = captured:sub(1, max_chars)
	end
	local lines = { "# Gator context bundle", "", "## Objective", redact.text(objective), "" }
	section(lines, "Captured source", table.concat({
		"- Path: `" .. capture.path .. "`",
		"- Lines: " .. capture.first_line .. "-" .. capture.last_line,
		"- Filetype: " .. capture.language,
		"",
		"```" .. capture.language,
		captured,
		"```",
	}, "\n"))
	section(lines, "Current diff", opts.diff)
	section(lines, "Handoff note", opts.note)
	section(lines, "Source-agent summary", opts.summary)
	if opts.transcript and profile == "full" then
		section(lines, "Gator-owned transcript", opts.transcript)
	end
	return table.concat(lines, "\n"), {
		input_tokens = M.estimate(objective .. "\n" .. captured .. "\n" .. (opts.diff or "") .. "\n" .. (opts.note or "")),
		state = "estimated",
	}
end

return M
