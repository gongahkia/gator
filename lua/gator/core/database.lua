local errors = require("gator.error")
local redact = require("gator.policy.redact")
local run = require("gator.core.run")
local task = require("gator.core.task")
local M = { schema_version = 4, evidence_excerpt_max_bytes = 4096 }
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
		"INSERT INTO tasks (id, record_json, created_at, updated_at) VALUES ("
			.. quote(record.id)
			.. ", "
			.. quote(vim.json.encode(record))
			.. ", "
			.. record.created_at
			.. ", "
			.. record.updated_at
			.. ") ON CONFLICT(id) DO UPDATE SET record_json = excluded.record_json, created_at = excluded.created_at, updated_at = excluded.updated_at;",
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

return M
