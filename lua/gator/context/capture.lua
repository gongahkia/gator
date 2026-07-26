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
	local first_line, last_line = opts.first_line, opts.last_line
	if first_line == nil and last_line == nil then
		first_line, last_line = 1, line_count
	elseif first_line == nil or last_line == nil then
		fail("line range must provide both first_line and last_line")
	end
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
	local result = vim.system({ "git", "diff", "--no-ext-diff", "HEAD", "--", "." }, { cwd = root, text = true }):wait()
	if result.code ~= 0 then
		result = vim.system({ "git", "diff", "--no-ext-diff", "--", "." }, { cwd = root, text = true }):wait()
	end
	if result.code ~= 0 then
		return nil
	end
	local value = vim.trim(result.stdout or "")
	return value ~= "" and redact.text(value) or nil
end

local function relative(path)
	return type(path) == "string"
		and path ~= ""
		and not path:find("%z")
		and path:sub(1, 1) ~= "/"
		and not path:match("^%.%./")
		and not path:find("/../", 1, true)
end

local function changed_paths(root)
	local result, seen = {}, {}
	for _, argv in ipairs({
		{ "git", "diff", "--name-only", "HEAD", "--", "." },
		{ "git", "ls-files", "--others", "--exclude-standard" },
	}) do
		local process = vim.system(argv, { cwd = root, text = true }):wait()
		if process.code == 0 then
			for _, path in ipairs(vim.split(process.stdout or "", "\n", { plain = true, trimempty = true })) do
				if relative(path) and not seen[path] then
					seen[path] = true
					table.insert(result, path)
				end
			end
		end
	end
	table.sort(result)
	return result
end

local function read(path, maximum)
	local stat = vim.uv.fs_stat(path)
	if not stat then
		return nil, "deleted"
	end
	if stat.type ~= "file" then
		return nil, "not a regular file"
	end
	if stat.size > maximum then
		return nil, "exceeds configured limit"
	end
	local handle = vim.uv.fs_open(path, "r", 420)
	if not handle then
		return nil, "unreadable"
	end
	local value = vim.uv.fs_read(handle, stat.size, 0)
	vim.uv.fs_close(handle)
	if type(value) ~= "string" then
		return nil, "unreadable"
	end
	if value:find("%z") then
		return nil, "binary"
	end
	return value
end

function M.snapshot(root, opts)
	root = text(root, "project root")
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("snapshot options must be an object")
	end
	local max_files, max_chars = opts.max_files or 24, opts.max_file_chars or 65536
	if
		type(max_files) ~= "number"
		or max_files < 0
		or max_files % 1 ~= 0
		or type(max_chars) ~= "number"
		or max_chars < 0
		or max_chars % 1 ~= 0
	then
		fail("snapshot limits must be non-negative integers")
	end
	local files, remaining = {}, max_chars
	for index, path in ipairs(changed_paths(root)) do
		local entry = { path = path }
		if max_files == 0 or max_chars == 0 then
			entry.state, entry.reason = "omitted", "snapshot file copies disabled"
		elseif index > max_files then
			entry.state, entry.reason = "omitted", "exceeds configured file count"
		else
			local value, reason = read(root .. "/" .. path, remaining)
			if value then
				entry.state, entry.content, entry.apply_content, entry.bytes =
					"included", redact.text(value), value, #value
				remaining = remaining - #value
			else
				entry.state, entry.reason = reason == "deleted" and "deleted" or "omitted", reason
			end
		end
		table.insert(files, entry)
	end
	return { root = root, diff = M.diff(root), files = files }
end

function M.snapshot_markdown(value)
	if type(value) ~= "table" or type(value.files) ~= "table" then
		fail("snapshot must contain files")
	end
	local lines = { "## Handoff file snapshot", "" }
	if #value.files == 0 then
		table.insert(lines, "- No changed or untracked files captured.")
	else
		for _, file in ipairs(value.files) do
			local detail = file.state == "included" and ("included · " .. file.bytes .. " bytes")
				or (file.state .. " · " .. (file.reason or ""))
			table.insert(lines, "- `" .. redact.text(file.path) .. "` · " .. detail)
		end
	end
	if value.diff then
		vim.list_extend(lines, { "", "## Current source diff", "", "```diff", value.diff, "```" })
	end
	return table.concat(lines, "\n")
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
	section(
		lines,
		"Captured source",
		table.concat({
			"- Path: `" .. capture.path .. "`",
			"- Lines: " .. capture.first_line .. "-" .. capture.last_line,
			"- Filetype: " .. capture.language,
			"",
			"```" .. capture.language,
			captured,
			"```",
		}, "\n")
	)
	section(lines, "Current diff", opts.diff)
	section(lines, "Handoff note", opts.note)
	section(lines, "Source-agent summary", opts.summary)
	if opts.transcript and profile == "full" then
		section(lines, "Gator-owned transcript", opts.transcript)
	end
	return table.concat(lines, "\n"),
		{
			input_tokens = M.estimate(
				objective .. "\n" .. captured .. "\n" .. (opts.diff or "") .. "\n" .. (opts.note or "")
			),
			state = "estimated",
		}
end

return M
