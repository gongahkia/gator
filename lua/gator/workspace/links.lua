local M = {}
local Store = {}

Store.__index = Store

local function fail(message)
	error("Gator workspace links: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
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
	if type(value.root) ~= "string" or value.root == "" then
		fail("worktree.root must be a non-empty string")
	end
	return { id = identifier(value.id, "worktree.id"), root = value.root }
end

local function link(value)
	if type(value) ~= "table" then
		fail("link must be a table")
	end
	for key in pairs(value) do
		if key ~= "task_id" and key ~= "run_id" and key ~= "worktree" then
			fail("link contains unsupported field: " .. tostring(key))
		end
	end
	return {
		task_id = identifier(value.task_id, "task_id"),
		run_id = identifier(value.run_id, "run_id"),
		worktree = worktree(value.worktree),
	}
end

local function same(left, right)
	return left.task_id == right.task_id
		and left.run_id == right.run_id
		and left.worktree.id == right.worktree.id
		and left.worktree.root == right.worktree.root
end

local function records(path)
	if vim.fn.filereadable(path) == 0 then
		return {}
	end
	local ok, document = pcall(vim.json.decode, table.concat(vim.fn.readfile(path), "\n"))
	if not ok or type(document) ~= "table" then
		fail("link file is not valid JSON: " .. path)
	end
	if document.schema_version ~= 1 or type(document.links) ~= "table" or not vim.islist(document.links) then
		fail("link file has an unsupported schema: " .. path)
	end
	local result = {}
	local runs, worktrees = {}, {}
	for index, value in ipairs(document.links) do
		result[index] = link(value)
		local record = result[index]
		if runs[record.run_id] then
			fail("link file duplicates run id: " .. record.run_id)
		end
		runs[record.run_id] = true
		if worktrees[record.worktree.id] and worktrees[record.worktree.id] ~= record.worktree.root then
			fail("link file maps a worktree id to multiple roots")
		end
		worktrees[record.worktree.id] = record.worktree.root
	end
	return result
end

local function write(path, values)
	local parent = vim.fn.fnamemodify(path, ":h")
	if vim.fn.mkdir(parent, "p") < 0 then
		fail("cannot create link directory: " .. parent)
	end
	local temporary = path .. ".tmp-" .. vim.uv.hrtime()
	local ok, err = pcall(vim.fn.writefile, { vim.json.encode({ schema_version = 1, links = values }) }, temporary)
	if not ok then
		fail("cannot write link file: " .. err)
	end
	local renamed, rename_err = vim.uv.fs_rename(temporary, path)
	if not renamed then
		vim.fn.delete(temporary)
		fail("cannot replace link file: " .. rename_err)
	end
end

local function sorted(values)
	table.sort(values)
	return values
end

local function task_record(values, task_id)
	local runs, worktrees = {}, {}
	for _, value in ipairs(values) do
		if value.task_id == task_id then
			runs[value.run_id] = true
			worktrees[value.worktree.id] = true
		end
	end
	if next(runs) == nil then
		return nil
	end
	local run_ids, worktree_ids = {}, {}
	for id in pairs(runs) do
		table.insert(run_ids, id)
	end
	for id in pairs(worktrees) do
		table.insert(worktree_ids, id)
	end
	return { id = task_id, run_ids = sorted(run_ids), worktree_ids = sorted(worktree_ids) }
end

local function run_record(values, run_id)
	for _, value in ipairs(values) do
		if value.run_id == run_id then
			return {
				id = value.run_id,
				task_id = value.task_id,
				worktree_id = value.worktree.id,
				worktree_root = value.worktree.root,
			}
		end
	end
	return nil
end

local function worktree_record(values, worktree_id)
	local tasks, runs, root = {}, {}, nil
	for _, value in ipairs(values) do
		if value.worktree.id == worktree_id then
			root = value.worktree.root
			tasks[value.task_id] = true
			runs[value.run_id] = true
		end
	end
	if root == nil then
		return nil
	end
	local task_ids, run_ids = {}, {}
	for id in pairs(tasks) do
		table.insert(task_ids, id)
	end
	for id in pairs(runs) do
		table.insert(run_ids, id)
	end
	return { id = worktree_id, root = root, task_ids = sorted(task_ids), run_ids = sorted(run_ids) }
end

function M.open(path)
	path = path or vim.fn.stdpath("state") .. "/gator/workspace-links.json"
	if type(path) ~= "string" or path == "" then
		fail("path must be a non-empty string")
	end
	return setmetatable({ path = path }, Store)
end

function Store:link(value)
	local candidate = link(value)
	local values = records(self.path)
	for _, existing in ipairs(values) do
		if existing.run_id == candidate.run_id then
			if not same(existing, candidate) then
				fail("run is already linked to a different task or worktree")
			end
			return {
				task = task_record(values, candidate.task_id),
				run = run_record(values, candidate.run_id),
				worktree = worktree_record(values, candidate.worktree.id),
			}
		end
		if existing.worktree.id == candidate.worktree.id and existing.worktree.root ~= candidate.worktree.root then
			fail("worktree id is already linked to a different root")
		end
	end
	table.insert(values, candidate)
	table.sort(values, function(left, right)
		return left.run_id < right.run_id
	end)
	write(self.path, values)
	return {
		task = task_record(values, candidate.task_id),
		run = run_record(values, candidate.run_id),
		worktree = worktree_record(values, candidate.worktree.id),
	}
end

function Store:task(task_id)
	return task_record(records(self.path), identifier(task_id, "task_id"))
end

function Store:run(run_id)
	return run_record(records(self.path), identifier(run_id, "run_id"))
end

function Store:worktree(worktree_id)
	return worktree_record(records(self.path), identifier(worktree_id, "worktree_id"))
end

return M
