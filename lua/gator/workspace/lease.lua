local redact = require("gator.policy.redact")

local M = {}
local Store = {}

Store.__index = Store

local function fail(message)
	error("Gator workspace lease: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function timestamp(value, name)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail(name .. " must be a non-negative integer timestamp")
	end
	return value
end

local function root(value)
	if type(value) ~= "string" or value == "" or value:find("\0", 1, true) then
		fail("worktree.root must be non-empty text without NUL")
	end
	if redact.text(value) ~= value then
		fail("worktree.root contains sensitive text")
	end
	return value
end

local function worktree(value)
	if type(value) ~= "table" then
		fail("worktree must be a table")
	end
	for key in pairs(value) do
		if key ~= "id" and key ~= "root" then
			fail("worktree contains unsupported field: " .. tostring(key))
		end
	end
	return { id = identifier(value.id, "worktree.id"), root = root(value.root) }
end

local function lease(value)
	if type(value) ~= "table" then
		fail("lease must be a table")
	end
	for key in pairs(value) do
		if key ~= "worktree" and key ~= "run_id" and key ~= "acquired_at" and key ~= "expires_at" then
			fail("lease contains unsupported field: " .. tostring(key))
		end
	end
	local acquired_at, expires_at =
		timestamp(value.acquired_at, "lease.acquired_at"), timestamp(value.expires_at, "lease.expires_at")
	if expires_at <= acquired_at then
		fail("lease.expires_at must be after lease.acquired_at")
	end
	return {
		worktree = worktree(value.worktree),
		run_id = identifier(value.run_id, "lease.run_id"),
		acquired_at = acquired_at,
		expires_at = expires_at,
	}
end

local function writer_lock(value)
	if type(value) ~= "table" then
		fail("writer lock must be a table")
	end
	for key in pairs(value) do
		if key ~= "worktree_id" and key ~= "run_id" and key ~= "acquired_at" and key ~= "expires_at" then
			fail("writer lock contains unsupported field: " .. tostring(key))
		end
	end
	local acquired_at, expires_at =
		timestamp(value.acquired_at, "writer lock.acquired_at"), timestamp(value.expires_at, "writer lock.expires_at")
	if expires_at <= acquired_at then
		fail("writer lock.expires_at must be after writer lock.acquired_at")
	end
	return {
		worktree_id = identifier(value.worktree_id, "writer lock.worktree_id"),
		run_id = identifier(value.run_id, "writer lock.run_id"),
		acquired_at = acquired_at,
		expires_at = expires_at,
	}
end

local function empty()
	return { schema_version = 1, leases = {}, writer_locks = {} }
end

local function document(value)
	if
		type(value) ~= "table"
		or value.schema_version ~= 1
		or not vim.islist(value.leases)
		or not vim.islist(value.writer_locks)
	then
		fail("lease file has an unsupported schema")
	end
	local result, runs, worktrees = empty(), {}, {}
	for index, value in ipairs(value.leases) do
		local record = lease(value)
		if runs[record.run_id] then
			fail("lease file duplicates run id")
		end
		if worktrees[record.worktree.id] and worktrees[record.worktree.id] ~= record.worktree.root then
			fail("lease file maps a worktree id to multiple roots")
		end
		runs[record.run_id], worktrees[record.worktree.id] = record, record.worktree.root
		result.leases[index] = record
	end
	for index, value in ipairs(value.writer_locks) do
		local record = writer_lock(value)
		if
			result.writer_locks[index]
			or not runs[record.run_id]
			or runs[record.run_id].worktree.id ~= record.worktree_id
		then
			fail("writer lock has no matching lease")
		end
		for _, existing in ipairs(result.writer_locks) do
			if existing.worktree_id == record.worktree_id then
				fail("lease file duplicates writer lock worktree")
			end
		end
		result.writer_locks[index] = record
	end
	return result
end

local function read(path)
	if vim.fn.filereadable(path) == 0 then
		return empty()
	end
	local ok, value = pcall(function()
		return document(vim.json.decode(table.concat(vim.fn.readfile(path), "\n")))
	end)
	if not ok then
		return nil, "lease file is invalid"
	end
	return value
end

local function write(path, value)
	local temporary = path .. ".tmp-" .. vim.uv.hrtime()
	local ok, result = pcall(vim.fn.writefile, { vim.json.encode(value) }, temporary)
	if not ok or result ~= 0 then
		return false
	end
	local renamed = vim.uv.fs_rename(temporary, path)
	if not renamed then
		vim.fn.delete(temporary)
		return false
	end
	return true
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

local function outcome(state, fields)
	fields = fields or {}
	fields.state = state
	return fields
end

local function acquire_file_lock(path, callback)
	local parent = vim.fn.fnamemodify(path, ":h")
	if vim.fn.mkdir(parent, "p") < 0 then
		return outcome("failed", { failure = "cannot create lease directory" })
	end
	local lock_path = path .. ".lock"
	local handle, err = vim.uv.fs_open(lock_path, "wx", 384)
	if not handle then
		if tostring(err):find("EEXIST", 1, true) then
			return outcome("unavailable", { failure = "lease store is busy" })
		end
		return outcome("failed", { failure = "cannot lock lease store" })
	end
	local ok, value = pcall(callback)
	vim.uv.fs_close(handle)
	vim.uv.fs_unlink(lock_path)
	if not ok or type(value) ~= "table" then
		return outcome("failed", { failure = "lease store operation failed" })
	end
	return value
end

local function prune(value, at)
	for index = #value.leases, 1, -1 do
		if value.leases[index].expires_at <= at then
			table.remove(value.leases, index)
		end
	end
	local runs = {}
	for _, value in ipairs(value.leases) do
		runs[value.run_id] = value
	end
	for index = #value.writer_locks, 1, -1 do
		local lock = value.writer_locks[index]
		if lock.expires_at <= at or not runs[lock.run_id] or runs[lock.run_id].worktree.id ~= lock.worktree_id then
			table.remove(value.writer_locks, index)
		end
	end
end

local function by_run(value, run_id)
	for _, record in ipairs(value.leases) do
		if record.run_id == run_id then
			return record
		end
	end
end

local function by_worktree(value, worktree_id)
	for _, record in ipairs(value.writer_locks) do
		if record.worktree_id == worktree_id then
			return record
		end
	end
end

local function order(value)
	table.sort(value.leases, function(left, right)
		return left.run_id < right.run_id
	end)
	table.sort(value.writer_locks, function(left, right)
		return left.worktree_id < right.worktree_id
	end)
	return value
end

local function request(opts, action)
	if type(opts) ~= "table" then
		fail(action .. " requires options")
	end
	for key in pairs(opts) do
		if key ~= "worktree" and key ~= "run_id" and key ~= "at" and key ~= "ttl" and key ~= "cancelled" then
			fail(action .. " contains unsupported field: " .. tostring(key))
		end
	end
	if opts.cancelled ~= nil and type(opts.cancelled) ~= "function" then
		fail("cancelled must be a function")
	end
	local value = {
		worktree = worktree(opts.worktree),
		run_id = identifier(opts.run_id, "run_id"),
		at = timestamp(opts.at or os.time(), "at"),
		ttl = opts.ttl or 300,
		cancelled = opts.cancelled,
	}
	if type(value.ttl) ~= "number" or value.ttl < 1 or value.ttl % 1 ~= 0 then
		fail("ttl must be a positive integer duration")
	end
	return value
end

function M.open(path)
	path = path or vim.fn.stdpath("state") .. "/gator/workspace-leases.json"
	if type(path) ~= "string" or path == "" or path:find("\0", 1, true) then
		fail("path must be non-empty text without NUL")
	end
	return setmetatable({ path = path }, Store)
end

function Store:acquire(opts)
	local value = request(opts, "acquire")
	local stopped = cancelled(value.cancelled)
	if stopped == nil then
		return outcome("failed", { failure = "cancellation check failed" })
	end
	if stopped then
		return outcome("cancelled")
	end
	return acquire_file_lock(self.path, function()
		local document, reason = read(self.path)
		if not document then
			return outcome("failed", { failure = reason })
		end
		prune(document, value.at)
		local existing, lock = by_run(document, value.run_id), by_worktree(document, value.worktree.id)
		if
			existing and (existing.worktree.id ~= value.worktree.id or existing.worktree.root ~= value.worktree.root)
		then
			return outcome("failed", { failure = "run is leased to a different worktree" })
		end
		if lock and lock.run_id ~= value.run_id then
			return outcome("unavailable", { failure = "writer lock is held" })
		end
		local expires_at = value.at + value.ttl
		if existing then
			existing.acquired_at, existing.expires_at = value.at, expires_at
		else
			existing =
				{ worktree = value.worktree, run_id = value.run_id, acquired_at = value.at, expires_at = expires_at }
			table.insert(document.leases, existing)
		end
		if lock then
			lock.acquired_at, lock.expires_at = value.at, expires_at
		else
			lock = {
				worktree_id = value.worktree.id,
				run_id = value.run_id,
				acquired_at = value.at,
				expires_at = expires_at,
			}
			table.insert(document.writer_locks, lock)
		end
		local stopped = cancelled(value.cancelled)
		if stopped == nil then
			return outcome("failed", { failure = "cancellation check failed" })
		end
		if stopped then
			return outcome("cancelled")
		end
		if not write(self.path, order(document)) then
			return outcome("failed", { failure = "cannot persist lease" })
		end
		return outcome("completed", { lease = vim.deepcopy(existing), writer_lock = vim.deepcopy(lock) })
	end)
end

function Store:release(opts)
	if type(opts) ~= "table" then
		fail("release requires options")
	end
	for key in pairs(opts) do
		if key ~= "run_id" then
			fail("release contains unsupported field: " .. tostring(key))
		end
	end
	local run_id = identifier(opts.run_id, "run_id")
	return acquire_file_lock(self.path, function()
		local document, reason = read(self.path)
		if not document then
			return outcome("failed", { failure = reason })
		end
		local found = false
		for index = #document.leases, 1, -1 do
			if document.leases[index].run_id == run_id then
				table.remove(document.leases, index)
				found = true
			end
		end
		if not found then
			return outcome("unavailable", { failure = "lease is unavailable" })
		end
		for index = #document.writer_locks, 1, -1 do
			if document.writer_locks[index].run_id == run_id then
				table.remove(document.writer_locks, index)
			end
		end
		if not write(self.path, order(document)) then
			return outcome("failed", { failure = "cannot persist lease release" })
		end
		return outcome("completed", { released = true })
	end)
end

function Store:inspect(opts)
	if type(opts) ~= "table" then
		fail("inspect requires options")
	end
	for key in pairs(opts) do
		if key ~= "worktree_id" and key ~= "at" then
			fail("inspect contains unsupported field: " .. tostring(key))
		end
	end
	local worktree_id, at = identifier(opts.worktree_id, "worktree_id"), timestamp(opts.at or os.time(), "at")
	local document, reason = read(self.path)
	if not document then
		return outcome("failed", { failure = reason })
	end
	prune(document, at)
	local leases = {}
	for _, record in ipairs(document.leases) do
		if record.worktree.id == worktree_id then
			table.insert(leases, vim.deepcopy(record))
		end
	end
	return outcome("completed", { leases = leases, writer_lock = vim.deepcopy(by_worktree(document, worktree_id)) })
end

return M
