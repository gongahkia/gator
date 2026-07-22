local M = {}

local function fail(message)
	error("Gator workspace collisions: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function path(value, name)
	if type(value) ~= "string" or value == "" or value:sub(1, 1) == "/" or value:find("\0", 1, true) then
		fail(name .. " must be a non-empty relative path")
	end
	for component in value:gmatch("[^/]+") do
		if component == ".." then
			fail(name .. " must not leave its worktree")
		end
	end
	return value
end

local function patterns(value)
	if value == nil then
		return {}
	end
	if type(value) ~= "table" or not vim.islist(value) then
		fail("generated must be an array")
	end
	local result = {}
	for index, pattern in ipairs(value) do
		result[index] = path(pattern, "generated " .. index)
	end
	return result
end

local function worktrees(value)
	if type(value) ~= "table" or not vim.islist(value) then
		fail("worktrees must be an array")
	end
	local result, ids = {}, {}
	for index, worktree in ipairs(value) do
		if type(worktree) ~= "table" then
			fail("worktree " .. index .. " must be a table")
		end
		for key in pairs(worktree) do
			if key ~= "id" and key ~= "active" and key ~= "changed_paths" then
				fail("worktree " .. index .. " contains unsupported field: " .. tostring(key))
			end
		end
		local id = identifier(worktree.id, "worktree " .. index .. " id")
		if ids[id] then
			fail("worktree id is duplicated: " .. id)
		end
		if type(worktree.active) ~= "boolean" then
			fail("worktree " .. index .. " active must be boolean")
		end
		if type(worktree.changed_paths) ~= "table" or not vim.islist(worktree.changed_paths) then
			fail("worktree " .. index .. " changed_paths must be an array")
		end
		local changed, seen = {}, {}
		for path_index, value in ipairs(worktree.changed_paths) do
			local changed_path = path(value, "worktree " .. index .. " changed path " .. path_index)
			if not seen[changed_path] then
				seen[changed_path] = true
				table.insert(changed, changed_path)
			end
		end
		ids[id] = true
		result[index] = { id = id, active = worktree.active, changed_paths = changed }
	end
	return result
end

local function generated(path, values)
	for _, pattern in ipairs(values) do
		if vim.fn.match(path, vim.fn.glob2regpat(pattern)) >= 0 then
			return true
		end
	end
	return false
end

local function ids(value)
	local result = {}
	for id in pairs(value) do
		table.insert(result, id)
	end
	table.sort(result)
	return result
end

local function cancelled(callback)
	if callback == nil then
		return false
	end
	local ok, value = pcall(callback)
	if not ok or type(value) ~= "boolean" then
		return nil
	end
	return value
end

function M.detect(opts)
	if type(opts) ~= "table" then
		fail("detect requires options")
	end
	for key in pairs(opts) do
		if key ~= "worktrees" and key ~= "generated" and key ~= "cancelled" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	if opts.cancelled ~= nil and type(opts.cancelled) ~= "function" then
		fail("cancelled must be a function")
	end
	local stopped = cancelled(opts.cancelled)
	if stopped == nil then
		return { state = "failed", warnings = {}, failure = "cancellation check failed" }
	end
	if stopped then
		return { state = "cancelled", warnings = {} }
	end
	local artifacts = patterns(opts.generated)
	local paths = {}
	for _, worktree in ipairs(worktrees(opts.worktrees)) do
		if worktree.active then
			for _, changed_path in ipairs(worktree.changed_paths) do
				paths[changed_path] = paths[changed_path] or {}
				paths[changed_path][worktree.id] = true
			end
		end
	end
	local warnings = {}
	for changed_path, worktree_ids in pairs(paths) do
		local value = ids(worktree_ids)
		if generated(changed_path, artifacts) then
			table.insert(warnings, { kind = "generated", path = changed_path, worktree_ids = value })
		elseif #value > 1 then
			table.insert(warnings, { kind = "overlap", path = changed_path, worktree_ids = value })
		end
	end
	table.sort(warnings, function(left, right)
		return left.kind == right.kind and left.path < right.path or left.kind < right.kind
	end)
	return { state = "completed", warnings = warnings }
end

return M
