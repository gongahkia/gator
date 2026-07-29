local redact = require("gator.policy.redact")

local M = {}
local roots = {}

local function fail(message)
	error("Gator completion context: " .. redact.text(tostring(message)), 3)
end

local function directory(value)
	local resolved = type(value) == "string" and vim.uv.fs_realpath(value) or nil
	return resolved and vim.fn.isdirectory(resolved) == 1 and vim.fs.normalize(resolved) or nil
end

local function git_root(path)
	local cached = roots[path]
	if cached then
		return cached
	end
	local result = vim.system({ "git", "rev-parse", "--show-toplevel" }, { cwd = path, text = true }):wait()
	local root = result.code == 0 and directory(vim.trim(result.stdout or "")) or nil
	roots[path] = root or false
	return root
end

local function lsp_root(buffer)
	for _, client in ipairs(vim.lsp.get_clients({ bufnr = buffer })) do
		local folders = client.workspace_folders or {}
		for _, folder in ipairs(folders) do
			local root = directory(folder.name or (folder.uri and vim.uri_to_fname(folder.uri)))
			if root then
				return root
			end
		end
		local root = directory(client.config and client.config.root_dir)
		if root then
			return root
		end
	end
	return nil
end

local function marker_root(path, markers)
	local found = vim.fs.find(markers, { path = path, upward = true, limit = 1 })[1]
	return found and directory(vim.fs.dirname(found)) or nil
end

function M.root(opts)
	if type(opts) ~= "table" or type(opts.buffer) ~= "number" or type(opts.settings) ~= "table" then
		fail("root requires buffer and settings")
	end
	local path = vim.api.nvim_buf_get_name(opts.buffer)
	local directory_path = path ~= "" and directory(vim.fs.dirname(path)) or directory(vim.fn.getcwd())
	if not directory_path then
		return nil
	end
	local root_settings = opts.settings.root
	if root_settings.strategy == "lsp" then
		return lsp_root(opts.buffer) or git_root(directory_path) or marker_root(directory_path, root_settings.markers)
	end
	if root_settings.strategy == "markers" then
		return marker_root(directory_path, root_settings.markers) or git_root(directory_path)
	end
	return git_root(directory_path) or directory_path
end

local function range(first, last, maximum, lines)
	local value = table.concat(vim.list_slice(lines, first + 1, last), "\n")
	while #value > maximum and last > first + 1 do
		last = last - 1
		value = table.concat(vim.list_slice(lines, first + 1, last), "\n")
	end
	if #value > maximum then
		value = value:sub(1, maximum)
	end
	return first, last, value
end

local function references(root, paths, remaining)
	if not root or #paths == 0 or remaining < 1 then
		return {}, 0
	end
	local values, redactions = {}, 0
	for _, path in ipairs(paths) do
		local tracked = vim.system({ "git", "ls-files", "--error-unmatch", "--", path }, { cwd = root, text = true })
			:wait()
		local absolute = vim.uv.fs_realpath(root .. "/" .. path)
		local stat = absolute and vim.uv.fs_stat(absolute) or nil
		if tracked.code == 0 and stat and stat.type == "file" and stat.size <= remaining then
			local handle = vim.uv.fs_open(absolute, "r", 420)
			local raw = handle and vim.uv.fs_read(handle, stat.size, 0) or nil
			if handle then
				vim.uv.fs_close(handle)
			end
			if type(raw) == "string" and not raw:find("%z") then
				local inspected = redact.inspect(raw)
				if #inspected.text <= remaining then
					table.insert(values, { path = path, text = inspected.text })
					remaining = remaining - #inspected.text
					redactions = redactions + inspected.matches
				end
			end
		end
	end
	return values, redactions
end

function M.document(opts)
	if type(opts) ~= "table" or type(opts.buffer) ~= "number" or type(opts.settings) ~= "table" then
		fail("document requires buffer and settings")
	end
	local buffer = opts.buffer == 0 and vim.api.nvim_get_current_buf() or opts.buffer
	if not vim.api.nvim_buf_is_valid(buffer) or not vim.bo[buffer].modifiable then
		return nil, "buffer is unavailable"
	end
	local path = vim.api.nvim_buf_get_name(buffer)
	if path == "" or path:match("^%w+://") then
		return nil, "buffer must be a file"
	end
	local cursor = vim.api.nvim_win_get_cursor(0)
	if vim.api.nvim_get_current_buf() ~= buffer then
		return nil, "buffer is not current"
	end
	local row, column = cursor[1] - 1, cursor[2]
	local lines = vim.api.nvim_buf_get_lines(buffer, 0, -1, false)
	local settings = opts.settings.context
	local document_maximum = settings.mode == "workspace" and math.max(1, math.floor(settings.max_bytes / 2))
		or settings.max_bytes
	local first, last = 0, #lines
	if settings.mode == "bounded" then
		first = math.max(0, row - settings.before_lines)
		last = math.min(#lines, row + settings.after_lines + 1)
	end
	first, last, _ = range(first, last, document_maximum, lines)
	local selected = vim.list_slice(lines, first + 1, last)
	local raw = table.concat(selected, "\n")
	if #raw > document_maximum then
		raw = raw:sub(1, document_maximum)
	end
	local inspected = redact.inspect(raw)
	local root = M.root({ buffer = buffer, settings = opts.settings })
	local workspace_references, reference_redactions = {}, 0
	if settings.mode == "workspace" then
		workspace_references, reference_redactions =
			references(root, settings.references, settings.max_bytes - #inspected.text)
	end
	return {
		document = {
			uri = vim.uri_from_fname(vim.fs.normalize(path)),
			language = vim.bo[opts.buffer].filetype == "" and "text" or vim.bo[opts.buffer].filetype,
			text = inspected.text,
			version = vim.api.nvim_buf_get_changedtick(buffer),
			window = { first_line = first, last_line = last - 1, truncated = #raw < #table.concat(selected, "\n") },
			cursor = { line = row, byte_column = column },
		},
		workspace = { root = root, apply_to = opts.settings.root.apply_to, references = workspace_references },
		context = { mode = settings.mode, redactions = inspected.matches + reference_redactions },
	}
end

function M.reset()
	roots = {}
end

return M
