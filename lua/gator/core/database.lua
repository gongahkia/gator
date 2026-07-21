local errors = require("gator.error")
local task = require("gator.core.task")
local M = { schema_version = 2 }
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
	local result = vim.system({ "sqlite3", "-batch", self.path }, { stdin = sql, text = true }):wait()
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

return M
