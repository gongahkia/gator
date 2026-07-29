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

function M.document(opts)
	if type(opts) ~= "table" or type(opts.buffer) ~= "number" or type(opts.settings) ~= "table" then
		fail("document requires buffer and settings")
	end
	if not vim.api.nvim_buf_is_valid(opts.buffer) or not vim.bo[opts.buffer].modifiable then
		return nil, "buffer is unavailable"
	end
	local path = vim.api.nvim_buf_get_name(opts.buffer)
	if path == "" or path:match("^%w+://") then
		return nil, "buffer must be a file"
	end
	local cursor = vim.api.nvim_win_get_cursor(0)
	if vim.api.nvim_get_current_buf() ~= opts.buffer then
		return nil, "buffer is not current"
	end
	local row, column = cursor[1] - 1, cursor[2]
	local lines = vim.api.nvim_buf_get_lines(opts.buffer, 0, -1, false)
	local settings = opts.settings.context
	local first, last = 0, #lines
	if settings.mode == "bounded" then
		first = math.max(0, row - settings.before_lines)
		last = math.min(#lines, row + settings.after_lines + 1)
	end
	first, last, _ = range(first, last, settings.max_bytes, lines)
	local selected = vim.list_slice(lines, first + 1, last)
	local raw = table.concat(selected, "\n")
	if #raw > settings.max_bytes then
		raw = raw:sub(1, settings.max_bytes)
	end
	local inspected = redact.inspect(raw)
	local root = M.root({ buffer = opts.buffer, settings = opts.settings })
	return {
		document = {
			uri = vim.uri_from_fname(vim.fs.normalize(path)),
			language = vim.bo[opts.buffer].filetype == "" and "text" or vim.bo[opts.buffer].filetype,
			text = inspected.text,
			version = vim.api.nvim_buf_get_changedtick(opts.buffer),
			window = { first_line = first, last_line = last - 1, truncated = #raw < #table.concat(selected, "\n") },
			cursor = { line = row, byte_column = column },
		},
		workspace = { root = root, apply_to = opts.settings.root.apply_to },
		context = { mode = settings.mode, redactions = inspected.matches },
	}
end

function M.reset()
	roots = {}
end

return M
