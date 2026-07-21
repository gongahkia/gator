local filesystem = require("gator.core.filesystem")
local redact = require("gator.policy.redact")
local run = require("gator.core.run")
local task = require("gator.core.task")

local M = { api_version = 1, schema_version = 1 }
local Backend = {}

Backend.__index = Backend

local function fail(message)
	error("Gator JSON backend: " .. redact.text(tostring(message)), 3)
end

local function id(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function arrays(value)
	if type(value) ~= "table" or not vim.islist(value) then
		fail("JSON backend records must be arrays")
	end
	return value
end

local function document(value)
	if type(value) ~= "table" or value.schema_version ~= M.schema_version then
		fail("JSON backend document has an unsupported schema")
	end
	for _, key in ipairs({ "tasks", "runs", "evidence_excerpts", "operations" }) do
		arrays(value[key])
	end
	return value
end

local function default_document()
	return { schema_version = M.schema_version, tasks = {}, runs = {}, evidence_excerpts = {}, operations = {} }
end

local function task_record(value)
	local ok, entity = pcall(task.from_record, value)
	if not ok then
		fail("task record is invalid")
	end
	return task.to_record(entity)
end

local function run_record(value)
	local ok, entity = pcall(run.from_record, value)
	if not ok then
		fail("run record is invalid")
	end
	return run.to_record(entity)
end

function M.open(path, opts)
	if type(path) ~= "string" or path == "" then
		fail("path must be non-empty")
	end
	opts = opts or {}
	if type(opts) ~= "table" or (opts.filesystem ~= nil and not filesystem.is(opts.filesystem)) then
		fail("open requires optional filesystem boundary")
	end
	local value = setmetatable(
		{ path = path, filesystem = opts.filesystem or filesystem.new(), api_version = M.api_version },
		Backend
	)
	require("gator.core.storage").validate_json_backend(value)
	return value
end

function Backend:read()
	if not self.filesystem:readable(self.path) then
		return default_document()
	end
	local ok, value = pcall(vim.json.decode, self.filesystem:read(self.path))
	if not ok then
		fail("document is not valid JSON")
	end
	return vim.deepcopy(document(value))
end

function Backend:write(value)
	value = document(value)
	local parent = vim.fn.fnamemodify(self.path, ":h")
	if not self.filesystem:mkdir(parent) then
		fail("cannot create storage directory")
	end
	local temporary = self.path .. ".tmp-" .. vim.uv.hrtime()
	if not self.filesystem:write(temporary, vim.json.encode(value)) then
		fail("cannot write storage document")
	end
	if not self.filesystem:rename(temporary, self.path) then
		self.filesystem:remove(temporary)
		fail("cannot replace storage document")
	end
end

local function replace(values, record)
	for index, value in ipairs(values) do
		if value.id == record.id then
			values[index] = record
			return
		end
	end
	table.insert(values, record)
end

function Backend:put_task(value)
	local record = task_record(value)
	local data = self:read()
	replace(data.tasks, record)
	table.sort(data.tasks, function(a, b)
		return a.created_at == b.created_at and a.id < b.id or a.created_at < b.created_at
	end)
	self:write(data)
	return task.from_record(record)
end

function Backend:get_task(value)
	value = id(value, "task id")
	for _, record in ipairs(self:read().tasks) do
		if record.id == value then
			return task.from_record(task_record(record))
		end
	end
	return nil
end

function Backend:list_tasks()
	local result = {}
	for _, record in ipairs(self:read().tasks) do
		table.insert(result, task.from_record(task_record(record)))
	end
	return result
end

function Backend:query_tasks(opts)
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("task query must be a table")
	end
	local result = {}
	for _, value in ipairs(self:list_tasks()) do
		if
			(not opts.lifecycle or value.lifecycle == opts.lifecycle)
			and (not opts.updated_after or value.updated_at >= opts.updated_after)
			and (not opts.updated_before or value.updated_at <= opts.updated_before)
		then
			table.insert(result, value)
		end
	end
	if opts.limit then
		while #result > opts.limit do
			table.remove(result)
		end
	end
	return result
end

function Backend:append_run(value)
	local record = run_record(value)
	local data = self:read()
	for _, existing in ipairs(data.runs) do
		if existing.id == record.id then
			fail("run already exists")
		end
	end
	table.insert(data.runs, record)
	table.sort(data.runs, function(a, b)
		return a.id < b.id
	end)
	self:write(data)
	return run.from_record(record)
end

function Backend:get_run(value)
	value = id(value, "run id")
	for _, record in ipairs(self:read().runs) do
		if record.id == value then
			return run.from_record(run_record(record))
		end
	end
	return nil
end

function Backend:list_runs(task_id)
	local result = {}
	for _, value in ipairs(self:read().runs) do
		if not task_id or value.task_id == task_id then
			table.insert(result, run.from_record(run_record(value)))
		end
	end
	return result
end

function Backend:query_runs(opts)
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("run query must be a table")
	end
	local result = {}
	for _, value in ipairs(self:list_runs(opts.task_id)) do
		if
			(not opts.provider or value.provider.name == opts.provider)
			and (not opts.session_id or value.provider.session_id == opts.session_id)
			and (not opts.workspace_root or value.workspace.root == opts.workspace_root)
			and (not opts.state or value.state == opts.state)
			and (not opts.started_after or value.timing.started_at >= opts.started_after)
			and (not opts.started_before or value.timing.started_at <= opts.started_before)
		then
			table.insert(result, value)
		end
	end
	if opts.limit then
		while #result > opts.limit do
			table.remove(result)
		end
	end
	return result
end

function Backend:append_run_event(run_id, value)
	run_id = id(run_id, "run id")
	local event = run.event(value)
	if event.run_id ~= run_id then
		fail("event belongs to a different run")
	end
	local data = self:read()
	for _, record in ipairs(data.runs) do
		if record.id == run_id then
			for _, existing in ipairs(record.events) do
				if existing.id == event.id then
					fail("event already exists")
				end
			end
			table.insert(record.events, event)
			table.sort(record.events, function(a, b)
				return a.at == b.at and a.id < b.id or a.at < b.at
			end)
			self:write(data)
			return vim.deepcopy(event)
		end
	end
	fail("run is unavailable")
end

function Backend:append_evidence_excerpt(value)
	if type(value) ~= "table" or type(value.text) ~= "string" or value.text == "" then
		fail("evidence excerpt is invalid")
	end
	local record = {
		id = id(value.id, "evidence id"),
		task_id = id(value.task_id, "task id"),
		kind = id(value.kind, "evidence kind"),
		at = value.at,
		text = redact.text(value.text):sub(1, 4096),
	}
	if type(record.at) ~= "number" or record.at < 0 or record.at % 1 ~= 0 then
		fail("evidence timestamp is invalid")
	end
	if not self:get_task(record.task_id) then
		fail("task is unavailable")
	end
	local data = self:read()
	for _, existing in ipairs(data.evidence_excerpts) do
		if existing.id == record.id then
			fail("evidence already exists")
		end
	end
	table.insert(data.evidence_excerpts, record)
	self:write(data)
	return vim.deepcopy(record)
end

function Backend:list_evidence_excerpts(task_id)
	task_id = id(task_id, "task id")
	local result = {}
	for _, value in ipairs(self:read().evidence_excerpts) do
		if value.task_id == task_id then
			table.insert(result, vim.deepcopy(value))
		end
	end
	table.sort(result, function(a, b)
		return a.at == b.at and a.id < b.id or a.at < b.at
	end)
	return result
end

function Backend:delete_evidence_excerpt(value, confirm)
	value = id(value, "evidence id")
	if confirm ~= true then
		fail("evidence deletion requires confirmation")
	end
	local data = self:read()
	for index, record in ipairs(data.evidence_excerpts) do
		if record.id == value then
			table.remove(data.evidence_excerpts, index)
			self:write(data)
			return { id = value, deleted = true, verified = true }
		end
	end
	return false
end

function Backend:commit_task_operation(value)
	if type(value) ~= "table" or type(value.operation) ~= "table" then
		fail("task operation is invalid")
	end
	local record = task_record(value.task)
	local operation = {
		id = id(value.operation.id, "operation id"),
		task_id = record.id,
		kind = id(value.operation.kind, "operation kind"),
		at = value.operation.at,
	}
	if type(operation.at) ~= "number" or operation.at < 0 or operation.at % 1 ~= 0 then
		fail("operation timestamp is invalid")
	end
	local data = self:read()
	for _, existing in ipairs(data.operations) do
		if existing.id == operation.id then
			fail("operation already exists")
		end
	end
	replace(data.tasks, record)
	table.insert(data.operations, operation)
	self:write(data)
	return { task = task.from_record(record), operation = vim.deepcopy(operation) }
end

function Backend:list_task_operations(task_id)
	task_id = id(task_id, "task id")
	local result = {}
	for _, value in ipairs(self:read().operations) do
		if value.task_id == task_id then
			table.insert(result, vim.deepcopy(value))
		end
	end
	table.sort(result, function(a, b)
		return a.at == b.at and a.id < b.id or a.at < b.at
	end)
	return result
end

function Backend:preview_export(opts)
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("export preview is invalid")
	end
	local tasks = opts.task_id and (self:get_task(opts.task_id) and { self:get_task(opts.task_id) } or {})
		or self:list_tasks()
	local result = { schema_version = 1, tasks = {}, runs = {}, evidence_excerpts = {}, operations = {} }
	for _, value in ipairs(tasks) do
		local record = task.to_record(value)
		table.insert(result.tasks, record)
		for _, child in ipairs(self:list_runs(record.id)) do
			table.insert(result.runs, run.to_record(child))
		end
		vim.list_extend(result.evidence_excerpts, self:list_evidence_excerpts(record.id))
		vim.list_extend(result.operations, self:list_task_operations(record.id))
	end
	return result
end

function Backend:export_bundle(opts)
	return vim.json.encode(self:preview_export(opts))
end

return M
