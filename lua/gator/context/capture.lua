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
	if type(path) ~= "string" or path == "" or path:find("%z") or path:sub(1, 1) == "/" then
		return false
	end
	for segment in path:gmatch("[^/]+") do
		if segment == "." or segment == ".." then
			return false
		end
	end
	return true
end

local function git(root, argv)
	local result = vim.system(argv, { cwd = root, text = false }):wait()
	return result.code == 0 and (result.stdout or "") or nil
end

function M.head(root)
	root = text(root, "project root")
	local value = git(root, { "git", "rev-parse", "--verify", "HEAD" })
	return value and vim.trim(value) ~= "" and vim.trim(value) or nil
end

local function split_fields(value, count)
	local fields, rest = {}, value
	for _ = 1, count do
		local field, next_rest = rest:match("^([^ ]+) (.*)$")
		if not field then
			return nil
		end
		table.insert(fields, field)
		rest = next_rest
	end
	return fields, rest
end

local function status(root)
	local output = git(root, { "git", "status", "--porcelain=v2", "-z", "--renames", "--untracked-files=all" })
	if output == nil then
		fail("Git status is unavailable")
	end
	local records, index = {}, 1
	local values = vim.split(output, "\0", { plain = true, trimempty = true })
	while index <= #values do
		local value = values[index]
		local kind = value:sub(1, 1)
		if kind == "1" then
			local fields, path = split_fields(value:sub(3), 7)
			if fields and relative(path) then
				table.insert(records, {
					path = path,
					kind = "tracked",
					index_status = fields[1]:sub(1, 1),
					worktree_status = fields[1]:sub(2, 2),
					submodule = fields[2],
					modes = { head = fields[3], index = fields[4], worktree = fields[5] },
					hashes = { head = fields[6], index = fields[7] },
				})
			end
		elseif kind == "2" then
			local fields, path = split_fields(value:sub(3), 8)
			local source = values[index + 1]
			if fields and relative(path) and relative(source) then
				table.insert(records, {
					path = path,
					kind = fields[8]:sub(1, 1) == "C" and "copy" or "rename",
					rename_from = source,
					index_status = fields[1]:sub(1, 1),
					worktree_status = fields[1]:sub(2, 2),
					submodule = fields[2],
					modes = { head = fields[3], index = fields[4], worktree = fields[5] },
					hashes = { head = fields[6], index = fields[7] },
					score = fields[8],
				})
			end
			index = index + 1
		elseif kind == "u" then
			local fields, path = split_fields(value:sub(3), 9)
			if fields and relative(path) then
				table.insert(records, {
					path = path,
					kind = "unmerged",
					index_status = fields[1]:sub(1, 1),
					worktree_status = fields[1]:sub(2, 2),
					submodule = fields[2],
					modes = { stage1 = fields[3], stage2 = fields[4], stage3 = fields[5], worktree = fields[6] },
					hashes = { stage1 = fields[7], stage2 = fields[8], stage3 = fields[9] },
				})
			end
		elseif kind == "?" then
			local path = value:sub(3)
			if relative(path) then
				table.insert(records, {
					path = path,
					kind = "untracked",
					index_status = "?",
					worktree_status = "?",
				})
			end
		end
		index = index + 1
	end
	table.sort(records, function(left, right)
		return left.path < right.path
	end)
	return records
end

local function secret_like(path)
	local base = vim.fn.fnamemodify(path:lower(), ":t")
	return base == ".env"
		or base:match("^%.env[._-]") ~= nil
		or base:find("secret", 1, true) ~= nil
		or base:find("credential", 1, true) ~= nil
		or base:find("password", 1, true) ~= nil
		or base:find("token", 1, true) ~= nil
		or base:find("apikey", 1, true) ~= nil
		or base:find("api_key", 1, true) ~= nil
		or base:match("%.pem$") ~= nil
		or base:match("%.key$") ~= nil
		or base:match("^id_[a-z0-9_-]+$") ~= nil
end

local function read(path, maximum)
	local stat = vim.uv.fs_lstat(path)
	if not stat then
		return nil, "deleted"
	end
	if stat.type == "link" then
		return nil, "symbolic link"
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
	for index, change in ipairs(status(root)) do
		local entry = vim.tbl_extend("force", {
			path = change.path,
			kind = change.kind,
			index_status = change.index_status,
			worktree_status = change.worktree_status,
			staged = change.index_status and change.index_status ~= "." and change.index_status ~= "?" or false,
			unstaged = change.worktree_status and change.worktree_status ~= "." and change.worktree_status ~= "?"
				or false,
		}, change)
		if max_files == 0 or max_chars == 0 then
			entry.state, entry.reason = "omitted", "snapshot file copies disabled"
		elseif index > max_files then
			entry.state, entry.reason = "omitted", "exceeds configured file count"
		elseif entry.kind == "unmerged" then
			entry.state, entry.reason = "omitted", "unmerged Git path"
		elseif entry.submodule and entry.submodule:sub(1, 1) == "S" then
			entry.state, entry.reason = "omitted", "submodule"
		elseif entry.kind == "untracked" and secret_like(entry.path) then
			entry.state, entry.reason = "omitted", "secret-like untracked file"
		else
			local value, reason = read(root .. "/" .. entry.path, remaining)
			if value then
				entry.state, entry.content, entry.apply_content, entry.bytes, entry.content_sha256 =
					"included", redact.text(value), value, #value, vim.fn.sha256(value)
				remaining = remaining - #value
			else
				entry.state, entry.reason = reason == "deleted" and "deleted" or "omitted", reason
			end
		end
		table.insert(files, entry)
	end
	local diff = M.diff(root)
	return {
		schema_version = 1,
		root = root,
		base = { head = M.head(root) },
		diff = diff,
		diff_sha256 = diff and vim.fn.sha256(diff) or nil,
		files = files,
	}
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
			local status = "staged="
				.. tostring(file.staged == true)
				.. " · unstaged="
				.. tostring(file.unstaged == true)
			if file.rename_from then
				status = status .. " · from `" .. redact.text(file.rename_from) .. "`"
			end
			table.insert(lines, "- `" .. redact.text(file.path) .. "` · " .. detail .. " · " .. status)
		end
	end
	if value.diff then
		vim.list_extend(lines, { "", "## Current source diff", "", "```diff", value.diff, "```" })
	end
	return table.concat(lines, "\n")
end

function M.diagnostics(opts)
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("diagnostic options must be an object")
	end
	local buffer = opts.buffer or vim.api.nvim_get_current_buf()
	if type(buffer) ~= "number" or not vim.api.nvim_buf_is_valid(buffer) then
		fail("diagnostic buffer must be valid")
	end
	local first_line, last_line = opts.first_line or 1, opts.last_line or vim.api.nvim_buf_line_count(buffer)
	if
		type(first_line) ~= "number"
		or type(last_line) ~= "number"
		or first_line < 1
		or last_line < first_line
		or last_line > vim.api.nvim_buf_line_count(buffer)
	then
		fail("diagnostic line range is outside the buffer")
	end
	local values = {}
	for _, diagnostic in ipairs(vim.diagnostic.get(buffer)) do
		local line = (diagnostic.lnum or 0) + 1
		if line >= first_line and line <= last_line then
			table.insert(values, {
				line = line,
				column = (diagnostic.col or 0) + 1,
				severity = vim.diagnostic.severity[diagnostic.severity] or "UNKNOWN",
				message = redact.text(diagnostic.message or ""),
			})
		end
	end
	table.sort(values, function(left, right)
		return left.line == right.line and left.column < right.column or left.line < right.line
	end)
	if #values == 0 then
		fail("no diagnostics exist in the selected range")
	end
	local path = vim.api.nvim_buf_get_name(buffer)
	return {
		kind = "diagnostic",
		path = path ~= "" and path or "[unnamed buffer " .. buffer .. "]",
		first_line = first_line,
		last_line = last_line,
		values = values,
	}
end

function M.hunk(opts)
	if type(opts) ~= "table" then
		fail("hunk options must be an object")
	end
	local root = text(opts.root, "project root")
	local buffer = opts.buffer or vim.api.nvim_get_current_buf()
	if type(buffer) ~= "number" or not vim.api.nvim_buf_is_valid(buffer) then
		fail("hunk buffer must be valid")
	end
	local path = vim.api.nvim_buf_get_name(buffer)
	local resolved = path ~= "" and vim.uv.fs_realpath(path) or nil
	if not resolved or resolved:sub(1, #root + 1) ~= root .. "/" then
		fail("hunk buffer must be a file within the Git workspace")
	end
	local relative_path = resolved:sub(#root + 2)
	if not relative(relative_path) then
		fail("hunk path must be relative to the Git workspace")
	end
	local value = git(root, { "git", "diff", "--no-ext-diff", "--unified=3", "HEAD", "--", relative_path })
	if not value or vim.trim(value) == "" then
		fail("no Git diff exists for the current buffer")
	end
	return { kind = "hunk", path = relative_path, text = redact.text(value) }
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
