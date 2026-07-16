local M = {}

local function fail(message)
	error("Gator lexical context: " .. message, 3)
end

local function directory(value)
	local path = type(value) == "string" and vim.uv.fs_realpath(value) or nil
	if not path or vim.fn.isdirectory(path) ~= 1 then
		fail("cwd must be an existing directory")
	end
	return path
end

local function invoke(run, argv, cwd, name)
	local ok, result = pcall(run, argv, cwd)
	if not ok or type(result) ~= "table" or result.code ~= 0 or type(result.stdout) ~= "string" then
		fail(name .. " failed")
	end
	return result.stdout
end

local function native_signal(match)
	local buffer = vim.fn.bufnr(match.path, false)
	if buffer == -1 or not vim.api.nvim_buf_is_loaded(buffer) then
		return { available = false, reason = "file is not loaded" }
	end
	local ok, parser = pcall(vim.treesitter.get_parser, buffer)
	if not ok then
		return { available = false, reason = "Tree-sitter parser is unavailable" }
	end
	return { available = true, language = parser:lang() }
end

function M.search(opts)
	if type(opts) ~= "table" or type(opts.query) ~= "string" or opts.query == "" then
		fail("search requires a non-empty query")
	end
	if opts.run ~= nil and type(opts.run) ~= "function" then
		fail("run must be a function")
	end
	if opts.signal ~= nil and type(opts.signal) ~= "function" then
		fail("signal must be a function")
	end
	for key in pairs(opts) do
		if key ~= "cwd" and key ~= "query" and key ~= "run" and key ~= "signal" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	local cwd = directory(opts.cwd)
	local run = opts.run
		or function(argv, path)
			local result = vim.system(argv, { cwd = path, text = true }):wait()
			return { code = result.code, stdout = result.stdout or "" }
		end
	local root = vim.trim(invoke(run, { "git", "rev-parse", "--show-toplevel" }, cwd, "Git root lookup"))
	root = directory(root)
	local output = invoke(run, { "rg", "--json", "--fixed-strings", opts.query, "--" }, root, "ripgrep search")
	local result = {}
	for line in vim.gsplit(output, "\n", { plain = true, trimempty = true }) do
		local ok, event = pcall(vim.json.decode, line)
		if ok and type(event) == "table" and event.type == "match" and type(event.data) == "table" then
			local data = event.data
			if
				type(data.path) == "table"
				and type(data.path.text) == "string"
				and type(data.lines) == "table"
				and type(data.lines.text) == "string"
				and type(data.line_number) == "number"
			then
				local match = { path = root .. "/" .. data.path.text, line = data.line_number, text = data.lines.text }
				match.signal = opts.signal and opts.signal(match) or native_signal(match)
				table.insert(result, match)
			end
		end
	end
	return result
end

return M
