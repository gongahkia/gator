local M = {}

local function fail(message)
	error("Gator workspace switch: " .. message, 3)
end

local function directory(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be an existing directory")
	end
	local path = vim.uv.fs_realpath(value)
	if not path or vim.fn.isdirectory(path) ~= 1 then
		fail(name .. " must be an existing directory")
	end
	return path
end

local function path_in(root, path)
	return path:sub(1, #root + 1) == root .. "/"
end

local function plan(from, target)
	local rebound, skipped = {}, {}
	for _, window in ipairs(vim.api.nvim_tabpage_list_wins(0)) do
		local buffer = vim.api.nvim_win_get_buf(window)
		local name = vim.api.nvim_buf_get_name(buffer)
		local path = name == "" and nil or vim.uv.fs_realpath(name)
		if path and path_in(from, path) then
			if vim.api.nvim_get_option_value("modified", { buf = buffer }) then
				table.insert(skipped, { path = path, reason = "modified" })
			else
				local destination = target .. "/" .. path:sub(#from + 2)
				local stat = vim.uv.fs_stat(destination)
				if stat and stat.type == "file" then
					table.insert(rebound, { window = window, buffer = buffer, from = path, to = destination })
				else
					table.insert(skipped, { path = path, reason = "missing" })
				end
			end
		end
	end
	return rebound, skipped
end

local function set_cwd(path)
	local ok, err = pcall(vim.cmd, "tcd " .. vim.fn.fnameescape(path))
	if not ok then
		fail("cannot switch tab workspace: " .. tostring(err))
	end
end

function M.open(opts)
	if type(opts) ~= "table" then
		fail("open requires options")
	end
	for key in pairs(opts) do
		if key ~= "from" and key ~= "to" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	local from = directory(opts.from, "from")
	local target = directory(opts.to, "to")
	if from == target then
		fail("from and to must identify different workspaces")
	end
	local rebound, skipped = plan(from, target)
	local previous_cwd = vim.fn.getcwd()
	set_cwd(target)
	local applied = {}
	for _, change in ipairs(rebound) do
		local destination = vim.fn.bufadd(change.to)
		if destination < 1 then
			for _, previous in ipairs(applied) do
				pcall(vim.api.nvim_win_set_buf, previous.window, previous.buffer)
			end
			set_cwd(previous_cwd)
			fail("cannot add destination buffer: " .. change.to)
		end
		local ok = pcall(vim.api.nvim_win_set_buf, change.window, destination)
		if not ok then
			for _, previous in ipairs(applied) do
				pcall(vim.api.nvim_win_set_buf, previous.window, previous.buffer)
			end
			set_cwd(previous_cwd)
			fail("cannot rebind buffer: " .. change.from)
		end
		table.insert(applied, change)
	end
	local result = { from = from, to = target, rebound = {}, skipped = skipped }
	for index, change in ipairs(rebound) do
		result.rebound[index] = { from = change.from, to = change.to }
	end
	return result
end

return M
