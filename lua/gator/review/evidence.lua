local M = {}
local Store = {}
local kinds = { reviewer = true, test = true, approval = true, revision = true }

Store.__index = Store

local function fail(message)
	error("Gator review evidence: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function text(value, name)
	if type(value) ~= "string" or value == "" or value:find("\n", 1, true) then
		fail(name .. " must be a non-empty single-line string")
	end
	return value
end

local function timestamp(value, name)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail(name .. " must be a non-negative integer timestamp")
	end
	return value
end

local function event(value)
	if type(value) ~= "table" then
		fail("evidence must be a table")
	end
	for key in pairs(value) do
		if
			key ~= "task_id"
			and key ~= "kind"
			and key ~= "reviewer"
			and key ~= "command_id"
			and key ~= "output_ref"
			and key ~= "passed"
			and key ~= "approved"
			and key ~= "base_revision"
			and key ~= "head_revision"
			and key ~= "at"
		then
			fail("evidence contains unsupported field: " .. tostring(key))
		end
	end
	local kind = value.kind
	if type(kind) ~= "string" or not kinds[kind] then
		fail("evidence kind must be reviewer, test, approval, or revision")
	end
	local allowed = {
		reviewer = { task_id = true, kind = true, reviewer = true },
		test = { task_id = true, kind = true, command_id = true, output_ref = true, passed = true, at = true },
		approval = { task_id = true, kind = true, reviewer = true, approved = true, at = true },
		revision = { task_id = true, kind = true, base_revision = true, head_revision = true, at = true },
	}
	for key in pairs(value) do
		if not allowed[kind][key] then
			fail("evidence field is not valid for " .. kind .. ": " .. tostring(key))
		end
	end
	local result = { task_id = identifier(value.task_id, "task_id"), kind = kind }
	if kind == "reviewer" then
		result.reviewer = text(value.reviewer, "reviewer")
	elseif kind == "test" then
		result.command_id = identifier(value.command_id, "command_id")
		result.output_ref = text(value.output_ref, "output_ref")
		if type(value.passed) ~= "boolean" then
			fail("passed must be boolean")
		end
		result.passed = value.passed
		result.at = timestamp(value.at, "at")
	elseif kind == "approval" then
		result.reviewer = text(value.reviewer, "reviewer")
		if type(value.approved) ~= "boolean" then
			fail("approved must be boolean")
		end
		result.approved = value.approved
		result.at = timestamp(value.at, "at")
	else
		result.base_revision = text(value.base_revision, "base_revision")
		result.head_revision = text(value.head_revision, "head_revision")
		result.at = timestamp(value.at, "at")
	end
	return result
end

local function records(path)
	if vim.fn.filereadable(path) == 0 then
		return {}
	end
	local ok, document = pcall(vim.json.decode, table.concat(vim.fn.readfile(path), "\n"))
	if not ok or type(document) ~= "table" then
		fail("evidence file is not valid JSON: " .. path)
	end
	if document.schema_version ~= 1 or type(document.events) ~= "table" or not vim.islist(document.events) then
		fail("evidence file has an unsupported schema: " .. path)
	end
	local result = {}
	for index, value in ipairs(document.events) do
		result[index] = event(value)
	end
	return result
end

local function write(path, values)
	local parent = vim.fn.fnamemodify(path, ":h")
	if vim.fn.mkdir(parent, "p") < 0 then
		fail("cannot create evidence directory: " .. parent)
	end
	local temporary = path .. ".tmp-" .. vim.uv.hrtime()
	local ok, err = pcall(vim.fn.writefile, { vim.json.encode({ schema_version = 1, events = values }) }, temporary)
	if not ok then
		fail("cannot write evidence file: " .. err)
	end
	local renamed, rename_err = vim.uv.fs_rename(temporary, path)
	if not renamed then
		vim.fn.delete(temporary)
		fail("cannot replace evidence file: " .. rename_err)
	end
end

function M.open(path)
	path = path or vim.fn.stdpath("state") .. "/gator/review-evidence.json"
	if type(path) ~= "string" or path == "" then
		fail("path must be a non-empty string")
	end
	return setmetatable({ path = path }, Store)
end

function Store:record(attrs)
	if type(attrs) ~= "table" then
		fail("record requires evidence")
	end
	if attrs.at == nil and attrs.kind ~= "reviewer" then
		attrs = vim.tbl_extend("force", attrs, { at = os.time() })
	end
	local value = event(attrs)
	local values = records(self.path)
	if value.kind == "reviewer" then
		for _, existing in ipairs(values) do
			if
				existing.kind == "reviewer"
				and existing.task_id == value.task_id
				and existing.reviewer == value.reviewer
			then
				return self:task(value.task_id)
			end
		end
	end
	table.insert(values, value)
	write(self.path, values)
	return self:task(value.task_id)
end

function Store:task(task_id)
	task_id = identifier(task_id, "task_id")
	local result = { task_id = task_id, reviewers = {}, tests = {}, approvals = {}, revisions = {} }
	local reviewers, found = {}, false
	for _, value in ipairs(records(self.path)) do
		if value.task_id == task_id then
			found = true
			if value.kind == "reviewer" then
				reviewers[value.reviewer] = true
			elseif value.kind == "test" then
				table.insert(result.tests, vim.deepcopy(value))
			elseif value.kind == "approval" then
				reviewers[value.reviewer] = true
				table.insert(result.approvals, vim.deepcopy(value))
			else
				table.insert(result.revisions, vim.deepcopy(value))
			end
		end
	end
	if not found then
		return nil
	end
	for reviewer in pairs(reviewers) do
		table.insert(result.reviewers, reviewer)
	end
	table.sort(result.reviewers)
	return result
end

return M
