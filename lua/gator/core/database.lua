local errors = require("gator.error")
local M = { schema_version = 1 }
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
	if version ~= 0 then
		fail("database schema version is unsupported: " .. version)
	end
	legacy_dir = legacy_dir or vim.fn.fnamemodify(self.path, ":h")
	if type(legacy_dir) ~= "string" or legacy_dir == "" then
		fail("legacy state directory must be a non-empty string")
	end
	local sessions = read_legacy(legacy_dir .. "/sessions.json", "sessions")
	local threads = read_legacy(legacy_dir .. "/threads.json", "threads")
	local runs = read_legacy(legacy_dir .. "/runs.json", "runs")
	local statements = {
		"BEGIN IMMEDIATE;",
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
	table.insert(statements, "PRAGMA user_version = " .. M.schema_version .. ";")
	table.insert(statements, "COMMIT;")
	self:exec(table.concat(statements, "\n"))
	return true
end

return M
