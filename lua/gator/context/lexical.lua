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

function M.search_async(opts)
	if type(opts) ~= "table" or type(opts.query) ~= "string" or opts.query == "" then
		fail("search_async requires a non-empty query")
	end
	for key in pairs(opts) do
		if
			key ~= "cwd"
			and key ~= "query"
			and key ~= "spawn"
			and key ~= "signal"
			and key ~= "schedule"
			and key ~= "batch_size"
			and key ~= "on_result"
			and key ~= "on_error"
		then
			fail("async options contain unsupported field: " .. tostring(key))
		end
	end
	if type(opts.on_result) ~= "function" or type(opts.on_error) ~= "function" then
		fail("search_async requires on_result and on_error callbacks")
	end
	if opts.spawn ~= nil and type(opts.spawn) ~= "function" then
		fail("spawn must be a function")
	end
	if opts.signal ~= nil and type(opts.signal) ~= "function" then
		fail("signal must be a function")
	end
	local schedule = opts.schedule or vim.schedule
	if type(schedule) ~= "function" then
		fail("schedule must be a function")
	end
	local batch_size = opts.batch_size or 100
	if type(batch_size) ~= "number" or batch_size < 1 or batch_size % 1 ~= 0 then
		fail("batch_size must be a positive integer")
	end
	local cwd = directory(opts.cwd)
	local spawn = opts.spawn
		or function(argv, path, callback)
			return vim.system(argv, { cwd = path, text = true }, function(value)
				schedule(function()
					callback({ code = value.code, stdout = value.stdout or "" })
				end)
			end)
		end
	local function request(argv, path, callback, name)
		local ok, handle = pcall(spawn, argv, path, callback)
		if not ok then
			opts.on_error(name .. " failed")
			return nil
		end
		return handle
	end
	local function scan(root, output)
		local lines, result = {}, {}
		for line in vim.gsplit(output, "\n", { plain = true, trimempty = true }) do
			table.insert(lines, line)
		end
		local index = 1
		local function drain()
			local last = math.min(index + batch_size - 1, #lines)
			while index <= last do
				local ok, value = pcall(vim.json.decode, lines[index])
				if ok and type(value) == "table" and value.type == "match" and type(value.data) == "table" then
					local data = value.data
					if
						type(data.path) == "table"
						and type(data.path.text) == "string"
						and type(data.lines) == "table"
						and type(data.lines.text) == "string"
						and type(data.line_number) == "number"
					then
						local match =
							{ path = root .. "/" .. data.path.text, line = data.line_number, text = data.lines.text }
						match.signal = opts.signal and opts.signal(match) or native_signal(match)
						table.insert(result, match)
					end
				end
				index = index + 1
			end
			if index <= #lines then
				schedule(drain)
			else
				opts.on_result(result)
			end
		end
		schedule(drain)
	end
	return request({ "git", "rev-parse", "--show-toplevel" }, cwd, function(root)
		if type(root) ~= "table" or root.code ~= 0 or type(root.stdout) ~= "string" then
			opts.on_error("Git root lookup failed")
			return
		end
		local ok, path = pcall(directory, vim.trim(root.stdout))
		if not ok then
			opts.on_error("Git root lookup failed")
			return
		end
		request({ "rg", "--json", "--fixed-strings", opts.query, "--" }, path, function(search)
			if type(search) ~= "table" or search.code ~= 0 or type(search.stdout) ~= "string" then
				opts.on_error("ripgrep search failed")
				return
			end
			scan(path, search.stdout)
		end, "ripgrep search")
	end, "Git root lookup")
end

return M
