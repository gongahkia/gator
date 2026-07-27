local M = {}

local cache = {}
local cache_ms = 1000

local function fail(message)
	error("Gator resources: " .. tostring(message), 3)
end

local function integer(value, name)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail(name .. " must be a non-negative integer")
	end
	return value
end

local function scan(path)
	local handle, err = vim.uv.fs_scandir(path)
	if not handle then
		return nil, err
	end
	local bytes = 0
	while true do
		local name, kind = vim.uv.fs_scandir_next(handle)
		if not name then
			break
		end
		local child = path .. "/" .. name
		if kind == "directory" then
			local nested, nested_error = scan(child)
			if nested == nil then
				return nil, nested_error
			end
			bytes = bytes + nested
		elseif kind == "file" then
			local stat = vim.uv.fs_lstat(child)
			if stat and stat.type == "file" then
				bytes = bytes + stat.size
			end
		end
	end
	return bytes
end

function M.bytes(value)
	integer(value, "bytes")
	if value < 1024 then
		return value .. " B"
	end
	if value < 1024 * 1024 then
		return string.format("%.1f KiB", value / 1024)
	end
	if value < 1024 * 1024 * 1024 then
		return string.format("%.1f MiB", value / (1024 * 1024))
	end
	return string.format("%.1f GiB", value / (1024 * 1024 * 1024))
end

function M.duration(value)
	integer(value, "duration")
	if value < 60 then
		return value .. "s"
	end
	if value < 60 * 60 then
		return math.floor(value / 60) .. "m " .. value % 60 .. "s"
	end
	return math.floor(value / 3600) .. "h " .. math.floor(value % 3600 / 60) .. "m"
end

function M.worktree(path)
	if type(path) ~= "string" or path == "" then
		fail("worktree path must be non-empty text")
	end
	local root = vim.uv.fs_realpath(path)
	if not root or vim.fn.isdirectory(root) ~= 1 then
		return { state = "unknown" }
	end
	local now = vim.uv.now()
	local cached = cache[root]
	if cached and now - cached.at < cache_ms then
		return vim.deepcopy(cached.value)
	end
	local bytes = scan(root)
	local value = bytes == nil and { state = "unknown" } or { state = "measured", bytes = bytes }
	cache[root] = { at = now, value = value }
	return vim.deepcopy(value)
end

function M.summary(runs)
	if type(runs) ~= "table" or not vim.islist(runs) then
		fail("runs must be an array")
	end
	local paths, count, bytes, unknown = {}, 0, 0, false
	for _, run in ipairs(runs) do
		if type(run) == "table" and type(run.workspace) == "table" and run.workspace.kind == "worktree" then
			local path = run.workspace.root
			if type(path) == "string" and not paths[path] then
				paths[path] = true
				count = count + 1
				local value = M.worktree(path)
				if value.state == "measured" then
					bytes = bytes + value.bytes
				else
					unknown = true
				end
			end
		end
	end
	return { count = count, state = unknown and "partial" or "measured", bytes = bytes }
end

function M.run(value, now)
	if type(value) ~= "table" or type(value.resources) ~= "table" then
		fail("run resources are required")
	end
	now = integer(now, "now")
	local resources = value.resources
	local started_at = integer(resources.started_at, "run.resources.started_at")
	local finished_at = resources.finished_at == nil and now
		or integer(resources.finished_at, "run.resources.finished_at")
	return {
		wall_seconds = math.max(0, finished_at - started_at),
		context_bytes = integer(resources.context_bytes, "run.resources.context_bytes"),
		context_sends = integer(resources.context_sends, "run.resources.context_sends"),
		worktree = value.workspace.kind == "worktree" and M.worktree(value.workspace.root) or nil,
	}
end

return M
