local errors = require("gator.error")
local redact = require("gator.policy.redact")
local run = require("gator.core.run")
local task = require("gator.core.task")
local M = { schema_version = 6, export_schema_version = 1, evidence_excerpt_max_bytes = 4096 }
local Database = {}

Database.__index = Database

local function fail(detail)
	errors.raise(errors.new("database.migration_failed", "SQLite migration failed", {
		detail = detail,
		remedy = "Keep the existing local state files, correct the reported problem, then rerun migration.",
	}))
end

local function quote(value)
	return "'" .. value:gsub("'", "''") .. "'"
end

local function read_legacy(path, field)
	if vim.fn.filereadable(path) == 0 then
		return {}
	end
	local ok, document = pcall(vim.json.decode, table.concat(vim.fn.readfile(path), "\n"))
	if not ok or type(document) ~= "table" or type(document[field]) ~= "table" or not vim.islist(document[field]) then
		fail("legacy state has invalid schema: " .. path)
	end
	return document[field]
end

local function task_id(value)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail("task id must be a lowercase identifier")
	end
	return value
end

local function task_record(value)
	local ok, entity = pcall(task.from_record, value)
	if not ok then
		fail("task record is invalid")
	end
	return task.to_record(entity)
end

local function decode_task(value)
	local ok, record = pcall(vim.json.decode, value)
	if not ok or type(record) ~= "table" then
		fail("task record has invalid JSON")
	end
	return task.from_record(task_record(record))
end

local function run_id(value)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail("run id must be a lowercase identifier")
	end
	return value
end

local function run_record(value)
	local ok, entity = pcall(run.from_record, value)
	if not ok then
		fail("run record is invalid")
	end
	return run.to_record(entity)
end

local function event_record(value, expected_run_id)
	local ok, event = pcall(run.event, value)
	if not ok or event.run_id ~= expected_run_id then
		fail("run event is invalid")
	end
	return event
end

local function decode_run(value, events)
	local ok, record = pcall(vim.json.decode, value)
	if not ok or type(record) ~= "table" then
		fail("run record has invalid JSON")
	end
	record = run_record(record)
	record.events = events
	return run.from_record(record)
end

local function decode_event(value, expected_run_id)
	local ok, record = pcall(vim.json.decode, value)
	if not ok or type(record) ~= "table" then
		fail("run event has invalid JSON")
	end
	return event_record(record, expected_run_id)
end

local function event_insert(value)
	return "INSERT INTO run_events (id, run_id, at, record_json) VALUES ("
		.. quote(value.id)
		.. ", "
		.. quote(value.run_id)
		.. ", "
		.. value.at
		.. ", "
		.. quote(vim.json.encode(value))
		.. ");"
end

local function excerpt(value)
	if type(value) ~= "table" then
		fail("evidence excerpt must be a table")
	end
	for key in pairs(value) do
		if key ~= "id" and key ~= "task_id" and key ~= "kind" and key ~= "text" and key ~= "at" then
			fail("evidence excerpt contains unsupported field: " .. tostring(key))
		end
	end
	local text = value.text
	if type(text) ~= "string" or text == "" then
		fail("evidence excerpt text must be non-empty")
	end
	if type(value.kind) ~= "string" or not value.kind:match("^[a-z][a-z0-9_-]*$") then
		fail("evidence excerpt kind must be a lowercase identifier")
	end
	if type(value.at) ~= "number" or value.at < 0 or value.at % 1 ~= 0 then
		fail("evidence excerpt at must be a non-negative integer timestamp")
	end
	return {
		id = run_id(value.id),
		task_id = task_id(value.task_id),
		kind = value.kind,
		text = redact.text(text):sub(1, M.evidence_excerpt_max_bytes),
		at = value.at,
	}
end

local function decode_excerpt(value)
	local ok, record = pcall(vim.json.decode, value)
	if not ok then
		fail("evidence excerpt has invalid JSON")
	end
	return excerpt(record)
end

local function operation(value, expected_task_id)
	if type(value) ~= "table" then
		fail("task operation must be a table")
	end
	for key in pairs(value) do
		if key ~= "id" and key ~= "task_id" and key ~= "kind" and key ~= "at" then
			fail("task operation contains unsupported field: " .. tostring(key))
		end
	end
	if value.task_id ~= nil and value.task_id ~= expected_task_id then
		fail("task operation belongs to a different task")
	end
	if type(value.kind) ~= "string" or not value.kind:match("^[a-z][a-z0-9_-]*$") then
		fail("task operation kind must be a lowercase identifier")
	end
	if type(value.at) ~= "number" or value.at < 0 or value.at % 1 ~= 0 then
		fail("task operation at must be a non-negative integer timestamp")
	end
	return { id = run_id(value.id), task_id = expected_task_id, kind = value.kind, at = value.at }
end

local function task_upsert(record)
	return "INSERT INTO tasks (id, record_json, created_at, updated_at) VALUES ("
		.. quote(record.id)
		.. ", "
		.. quote(vim.json.encode(record))
		.. ", "
		.. record.created_at
		.. ", "
		.. record.updated_at
		.. ") ON CONFLICT(id) DO UPDATE SET record_json = excluded.record_json, created_at = excluded.created_at, updated_at = excluded.updated_at;"
end

local function query(value, allowed, name)
	if value == nil then
		return {}
	end
	if type(value) ~= "table" then
		fail(name .. " must be a table")
	end
	for key in pairs(value) do
		if not allowed[key] then
			fail(name .. " contains unsupported field: " .. tostring(key))
		end
	end
	return value
end

local function query_time(value, name)
	if value ~= nil and (type(value) ~= "number" or value < 0 or value % 1 ~= 0) then
		fail(name .. " must be a non-negative integer timestamp")
	end
	return value
end

local function query_limit(value)
	if value ~= nil and (type(value) ~= "number" or value < 1 or value % 1 ~= 0) then
		fail("query limit must be a positive integer")
	end
	return value
end

local function migration_one(legacy_dir)
	legacy_dir = legacy_dir or ""
	if type(legacy_dir) ~= "string" or legacy_dir == "" then
		fail("legacy state directory must be a non-empty string")
	end
	local sessions = read_legacy(legacy_dir .. "/sessions.json", "sessions")
	local threads = read_legacy(legacy_dir .. "/threads.json", "threads")
	local runs = read_legacy(legacy_dir .. "/runs.json", "runs")
	local statements = {
		"CREATE TABLE task_evidence (task_id TEXT NOT NULL, ref TEXT NOT NULL, record_json TEXT NOT NULL, PRIMARY KEY (task_id, ref));",
		"CREATE TABLE session_metadata (task_id TEXT NOT NULL, provider TEXT NOT NULL, session_id TEXT NOT NULL, record_json TEXT NOT NULL, PRIMARY KEY (task_id, provider, session_id));",
		"CREATE TABLE threads (id TEXT PRIMARY KEY, task_id TEXT NOT NULL, record_json TEXT NOT NULL);",
		"CREATE TABLE runs (id TEXT PRIMARY KEY, task_id TEXT NOT NULL, record_json TEXT NOT NULL);",
	}
	for _, value in ipairs(sessions) do
		if
			type(value) ~= "table"
			or type(value.task_id) ~= "string"
			or type(value.provider) ~= "string"
			or type(value.id) ~= "string"
		then
			fail("legacy session metadata record is incomplete")
		end
		table.insert(
			statements,
			"INSERT INTO session_metadata VALUES ("
				.. quote(value.task_id)
				.. ", "
				.. quote(value.provider)
				.. ", "
				.. quote(value.id)
				.. ", "
				.. quote(vim.json.encode(value))
				.. ");"
		)
	end
	for _, value in ipairs(threads) do
		if type(value) ~= "table" or type(value.id) ~= "string" or type(value.task_id) ~= "string" then
			fail("legacy thread record is incomplete")
		end
		table.insert(
			statements,
			"INSERT INTO threads VALUES ("
				.. quote(value.id)
				.. ", "
				.. quote(value.task_id)
				.. ", "
				.. quote(vim.json.encode(value))
				.. ");"
		)
	end
	for _, value in ipairs(runs) do
		if type(value) ~= "table" or type(value.id) ~= "string" or type(value.task_id) ~= "string" then
			fail("legacy run record is incomplete")
		end
		table.insert(
			statements,
			"INSERT INTO runs VALUES ("
				.. quote(value.id)
				.. ", "
				.. quote(value.task_id)
				.. ", "
				.. quote(vim.json.encode(value))
				.. ");"
		)
	end
	return statements
end

local migrations = {
	[1] = migration_one,
	[2] = function()
		return {
			"CREATE TABLE tasks (id TEXT PRIMARY KEY, record_json TEXT NOT NULL, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL);",
			"CREATE INDEX tasks_updated_at ON tasks (updated_at, id);",
		}
	end,
	[3] = function()
		return {
			"CREATE TABLE run_events (id TEXT PRIMARY KEY, run_id TEXT NOT NULL, at INTEGER NOT NULL, record_json TEXT NOT NULL);",
			"CREATE INDEX run_events_by_run ON run_events (run_id, at, id);",
			"INSERT INTO run_events (id, run_id, at, record_json) SELECT json_extract(event.value, '$.id'), runs.id, json_extract(event.value, '$.at'), json(event.value) FROM runs, json_each(runs.record_json, '$.events') AS event;",
			"UPDATE runs SET record_json = json_set(record_json, '$.events', json('[]'));",
		}
	end,
	[4] = function()
		return {
			"CREATE TABLE evidence_excerpts (id TEXT PRIMARY KEY, task_id TEXT NOT NULL, kind TEXT NOT NULL, at INTEGER NOT NULL, record_json TEXT NOT NULL);",
			"CREATE INDEX evidence_excerpts_by_task ON evidence_excerpts (task_id, at, id);",
		}
	end,
	[5] = function()
		return {
			"CREATE INDEX task_evidence_by_task ON task_evidence (task_id, ref);",
			"CREATE INDEX session_metadata_by_session ON session_metadata (provider, session_id, task_id);",
			"CREATE INDEX threads_by_task ON threads (task_id, id);",
			"CREATE INDEX runs_by_task ON runs (task_id, id);",
			"CREATE INDEX runs_by_session ON runs (json_extract(record_json, '$.provider.name'), json_extract(record_json, '$.provider.session_id'), id);",
			"CREATE INDEX runs_by_workspace ON runs (json_extract(record_json, '$.workspace.root'), id);",
			"CREATE INDEX runs_by_started_at ON runs (json_extract(record_json, '$.timing.started_at'), id);",
			"CREATE INDEX run_events_by_time ON run_events (at, id);",
		}
	end,
	[6] = function()
		return {
			"CREATE TABLE task_operations (id TEXT PRIMARY KEY, task_id TEXT NOT NULL, kind TEXT NOT NULL, at INTEGER NOT NULL, record_json TEXT NOT NULL);",
			"CREATE INDEX task_operations_by_task ON task_operations (task_id, at, id);",
		}
	end,
}

function M.open(path)
	path = path or vim.fn.stdpath("state") .. "/gator/state.sqlite3"
	if type(path) ~= "string" or path == "" then
		fail("database path must be a non-empty string")
	end
	if vim.fn.executable("sqlite3") ~= 1 then
		fail("sqlite3 executable is unavailable")
	end
	return setmetatable({ path = path }, Database)
end

function Database:exec(sql)
	local parent = vim.fn.fnamemodify(self.path, ":h")
	if vim.fn.mkdir(parent, "p") < 0 then
		fail("cannot create database directory: " .. parent)
	end
	local result = vim.system({ "sqlite3", "-batch", "-bail", self.path }, { stdin = sql, text = true }):wait()
	if result.code ~= 0 then
		fail(result.stderr ~= "" and result.stderr or "sqlite3 exited " .. result.code)
	end
	return result.stdout
end

function Database:version()
	local value = self:exec("PRAGMA user_version;"):match("%d+")
	if not value then
		fail("sqlite3 did not return user_version")
	end
	return tonumber(value)
end

function Database:migrate(legacy_dir)
	local version = self:version()
	if version > M.schema_version then
		fail("database schema version is newer than this Gator build")
	end
	if version == M.schema_version then
		return false
	end
	local statements = { "BEGIN IMMEDIATE;" }
	while version < M.schema_version do
		local next_version = version + 1
		local migration = migrations[next_version]
		if not migration then
			fail("database schema version is unsupported: " .. version)
		end
		if next_version == 1 then
			vim.list_extend(statements, migration(legacy_dir or vim.fn.fnamemodify(self.path, ":h")))
		else
			vim.list_extend(statements, migration())
		end
		table.insert(statements, "PRAGMA user_version = " .. next_version .. ";")
		version = next_version
	end
	table.insert(statements, "COMMIT;")
	self:exec(table.concat(statements, "\n"))
	return true
end

function Database:put_task(value)
	local record = task_record(value)
	if self:version() ~= M.schema_version then
		fail("database schema must migrate before task writes")
	end
	self:exec(table.concat({
		"BEGIN IMMEDIATE;",
		task_upsert(record),
		"COMMIT;",
	}, "\n"))
	return task.from_record(record)
end

function Database:get_task(id)
	id = task_id(id)
	if self:version() ~= M.schema_version then
		fail("database schema must migrate before task reads")
	end
	local value = vim.trim(self:exec("SELECT record_json FROM tasks WHERE id = " .. quote(id) .. ";"))
	if value == "" then
		return nil
	end
	return decode_task(value)
end

function Database:list_tasks()
	if self:version() ~= M.schema_version then
		fail("database schema must migrate before task reads")
	end
	local result = {}
	for _, value in
		ipairs(vim.split(self:exec("SELECT record_json FROM tasks ORDER BY created_at, id;"), "\n", {
			trimempty = true,
		}))
	do
		table.insert(result, decode_task(value))
	end
	return result
end

function Database:query_tasks(opts)
	opts = query(opts, { lifecycle = true, updated_after = true, updated_before = true, limit = true }, "task query")
	if opts.lifecycle ~= nil and (type(opts.lifecycle) ~= "string" or not task.lifecycle[opts.lifecycle]) then
		fail("task query lifecycle is unavailable")
	end
	query_time(opts.updated_after, "task query updated_after")
	query_time(opts.updated_before, "task query updated_before")
	query_limit(opts.limit)
	if self:version() ~= M.schema_version then
		fail("database schema must migrate before task queries")
	end
	local where = {}
	if opts.lifecycle then
		table.insert(where, "json_extract(record_json, '$.lifecycle') = " .. quote(opts.lifecycle))
	end
	if opts.updated_after then
		table.insert(where, "updated_at >= " .. opts.updated_after)
	end
	if opts.updated_before then
		table.insert(where, "updated_at <= " .. opts.updated_before)
	end
	local statement = "SELECT record_json FROM tasks"
	if #where > 0 then
		statement = statement .. " WHERE " .. table.concat(where, " AND ")
	end
	statement = statement .. " ORDER BY updated_at, id"
	if opts.limit then
		statement = statement .. " LIMIT " .. opts.limit
	end
	local result = {}
	for _, value in ipairs(vim.split(self:exec(statement .. ";"), "\n", { trimempty = true })) do
		table.insert(result, decode_task(value))
	end
	return result
end

function Database:append_run(value)
	local record = run_record(value)
	if self:version() ~= M.schema_version then
		fail("database schema must migrate before run writes")
	end
	local events = vim.deepcopy(record.events)
	record.events = {}
	local statements = {
		"BEGIN IMMEDIATE;",
		"INSERT INTO runs (id, task_id, record_json) VALUES ("
			.. quote(record.id)
			.. ", "
			.. quote(record.task_id)
			.. ", "
			.. quote(vim.json.encode(record))
			.. ");",
	}
	for _, event in ipairs(events) do
		table.insert(statements, event_insert(event))
	end
	table.insert(statements, "COMMIT;")
	self:exec(table.concat(statements, "\n"))
	return run.from_record(vim.tbl_extend("force", record, { events = events }))
end

function Database:append_run_event(id, value)
	id = run_id(id)
	local event = event_record(value, id)
	if self:version() ~= M.schema_version then
		fail("database schema must migrate before run event writes")
	end
	if not self:get_run(id) then
		fail("run is unavailable: " .. id)
	end
	self:exec(table.concat({ "BEGIN IMMEDIATE;", event_insert(event), "COMMIT;" }, "\n"))
	return vim.deepcopy(event)
end

function Database:list_run_events(id)
	id = run_id(id)
	if self:version() ~= M.schema_version then
		fail("database schema must migrate before run event reads")
	end
	local result = {}
	for _, value in
		ipairs(
			vim.split(
				self:exec("SELECT record_json FROM run_events WHERE run_id = " .. quote(id) .. " ORDER BY at, id;"),
				"\n",
				{
					trimempty = true,
				}
			)
		)
	do
		table.insert(result, decode_event(value, id))
	end
	return result
end

function Database:get_run(id)
	id = run_id(id)
	if self:version() ~= M.schema_version then
		fail("database schema must migrate before run reads")
	end
	local value = vim.trim(self:exec("SELECT record_json FROM runs WHERE id = " .. quote(id) .. ";"))
	if value == "" then
		return nil
	end
	return decode_run(value, self:list_run_events(id))
end

function Database:list_runs(task_value)
	if task_value ~= nil then
		task_id(task_value)
	end
	if self:version() ~= M.schema_version then
		fail("database schema must migrate before run reads")
	end
	local query = "SELECT record_json FROM runs"
	if task_value then
		query = query .. " WHERE task_id = " .. quote(task_value)
	end
	query = query .. " ORDER BY id;"
	local result = {}
	for _, value in ipairs(vim.split(self:exec(query), "\n", { trimempty = true })) do
		local record = decode_run(value, {})
		table.insert(result, self:get_run(record.id))
	end
	return result
end

function Database:query_runs(opts)
	opts = query(opts, {
		task_id = true,
		provider = true,
		session_id = true,
		workspace_root = true,
		state = true,
		started_after = true,
		started_before = true,
		limit = true,
	}, "run query")
	if opts.task_id ~= nil then
		task_id(opts.task_id)
	end
	for _, key in ipairs({ "provider", "session_id", "workspace_root" }) do
		if opts[key] ~= nil and (type(opts[key]) ~= "string" or opts[key] == "") then
			fail("run query " .. key .. " must be non-empty text")
		end
	end
	if opts.state ~= nil and (type(opts.state) ~= "string" or not run.states[opts.state]) then
		fail("run query state is unavailable")
	end
	query_time(opts.started_after, "run query started_after")
	query_time(opts.started_before, "run query started_before")
	query_limit(opts.limit)
	if self:version() ~= M.schema_version then
		fail("database schema must migrate before run queries")
	end
	local where = {}
	if opts.task_id then
		table.insert(where, "task_id = " .. quote(opts.task_id))
	end
	if opts.provider then
		table.insert(where, "json_extract(record_json, '$.provider.name') = " .. quote(opts.provider))
	end
	if opts.session_id then
		table.insert(where, "json_extract(record_json, '$.provider.session_id') = " .. quote(opts.session_id))
	end
	if opts.workspace_root then
		table.insert(where, "json_extract(record_json, '$.workspace.root') = " .. quote(opts.workspace_root))
	end
	if opts.state then
		table.insert(where, "json_extract(record_json, '$.state') = " .. quote(opts.state))
	end
	if opts.started_after then
		table.insert(where, "json_extract(record_json, '$.timing.started_at') >= " .. opts.started_after)
	end
	if opts.started_before then
		table.insert(where, "json_extract(record_json, '$.timing.started_at') <= " .. opts.started_before)
	end
	local statement = "SELECT record_json FROM runs"
	if #where > 0 then
		statement = statement .. " WHERE " .. table.concat(where, " AND ")
	end
	statement = statement .. " ORDER BY json_extract(record_json, '$.timing.started_at'), id"
	if opts.limit then
		statement = statement .. " LIMIT " .. opts.limit
	end
	local result = {}
	for _, value in ipairs(vim.split(self:exec(statement .. ";"), "\n", { trimempty = true })) do
		local record = decode_run(value, {})
		table.insert(result, self:get_run(record.id))
	end
	return result
end

function Database:append_evidence_excerpt(value)
	local record = excerpt(value)
	if self:version() ~= M.schema_version then
		fail("database schema must migrate before evidence writes")
	end
	if not self:get_task(record.task_id) then
		fail("task is unavailable: " .. record.task_id)
	end
	self:exec(table.concat({
		"BEGIN IMMEDIATE;",
		"INSERT INTO evidence_excerpts (id, task_id, kind, at, record_json) VALUES ("
			.. quote(record.id)
			.. ", "
			.. quote(record.task_id)
			.. ", "
			.. quote(record.kind)
			.. ", "
			.. record.at
			.. ", "
			.. quote(vim.json.encode(record))
			.. ");",
		"COMMIT;",
	}, "\n"))
	return vim.deepcopy(record)
end

function Database:list_evidence_excerpts(id)
	id = task_id(id)
	if self:version() ~= M.schema_version then
		fail("database schema must migrate before evidence reads")
	end
	local result = {}
	for _, value in
		ipairs(
			vim.split(
				self:exec(
					"SELECT record_json FROM evidence_excerpts WHERE task_id = " .. quote(id) .. " ORDER BY at, id;"
				),
				"\n",
				{ trimempty = true }
			)
		)
	do
		table.insert(result, decode_excerpt(value))
	end
	return result
end

function Database:delete_evidence_excerpt(id, confirm)
	id = run_id(id)
	if confirm ~= true then
		fail("evidence deletion requires explicit confirmation")
	end
	if self:version() ~= M.schema_version then
		fail("database schema must migrate before evidence deletion")
	end
	local existing = vim.trim(self:exec("SELECT id FROM evidence_excerpts WHERE id = " .. quote(id) .. ";"))
	if existing == "" then
		return false
	end
	self:exec(
		table.concat(
			{ "BEGIN IMMEDIATE;", "DELETE FROM evidence_excerpts WHERE id = " .. quote(id) .. ";", "COMMIT;" },
			"\n"
		)
	)
	if vim.trim(self:exec("SELECT id FROM evidence_excerpts WHERE id = " .. quote(id) .. ";")) ~= "" then
		fail("evidence deletion could not be verified")
	end
	return { id = id, deleted = true, verified = true }
end

function Database:commit_task_operation(value)
	if type(value) ~= "table" then
		fail("task operation transaction must be a table")
	end
	for key in pairs(value) do
		if key ~= "task" and key ~= "operation" then
			fail("task operation transaction contains unsupported field: " .. tostring(key))
		end
	end
	local record = task_record(value.task)
	local receipt = operation(value.operation, record.id)
	if self:version() ~= M.schema_version then
		fail("database schema must migrate before task operation writes")
	end
	self:exec(table.concat({
		"BEGIN IMMEDIATE;",
		task_upsert(record),
		"INSERT INTO task_operations (id, task_id, kind, at, record_json) VALUES ("
			.. quote(receipt.id)
			.. ", "
			.. quote(receipt.task_id)
			.. ", "
			.. quote(receipt.kind)
			.. ", "
			.. receipt.at
			.. ", "
			.. quote(vim.json.encode(receipt))
			.. ");",
		"COMMIT;",
	}, "\n"))
	return { task = task.from_record(record), operation = vim.deepcopy(receipt) }
end

function Database:list_task_operations(id)
	id = task_id(id)
	if self:version() ~= M.schema_version then
		fail("database schema must migrate before task operation reads")
	end
	local result = {}
	for _, value in
		ipairs(
			vim.split(
				self:exec(
					"SELECT record_json FROM task_operations WHERE task_id = " .. quote(id) .. " ORDER BY at, id;"
				),
				"\n",
				{ trimempty = true }
			)
		)
	do
		local ok, record = pcall(vim.json.decode, value)
		if not ok then
			fail("task operation has invalid JSON")
		end
		table.insert(result, operation(record, id))
	end
	return result
end

function Database:preview_export(opts)
	opts = query(opts, { task_id = true }, "export preview")
	if opts.task_id ~= nil then
		task_id(opts.task_id)
	end
	if self:version() ~= M.schema_version then
		fail("database schema must migrate before export previews")
	end
	local tasks = opts.task_id and (self:get_task(opts.task_id) and { self:get_task(opts.task_id) } or {})
		or self:list_tasks()
	local records =
		{ schema_version = M.export_schema_version, tasks = {}, runs = {}, evidence_excerpts = {}, operations = {} }
	for _, value in ipairs(tasks) do
		local task_value = task.to_record(value)
		table.insert(records.tasks, task_value)
		for _, run_value in ipairs(self:query_runs({ task_id = task_value.id })) do
			table.insert(records.runs, run.to_record(run_value))
		end
		vim.list_extend(records.evidence_excerpts, self:list_evidence_excerpts(task_value.id))
		vim.list_extend(records.operations, self:list_task_operations(task_value.id))
	end
	return vim.deepcopy(records)
end

function Database:export_bundle(opts)
	return vim.json.encode(self:preview_export(opts))
end

return M
