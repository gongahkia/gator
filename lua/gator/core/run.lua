local errors = require("gator.error")
local filesystem = require("gator.core.filesystem")
local redact = require("gator.policy.redact")
local M = {}
local Run = {}
local Store = {}

Run.__index = Run
Store.__index = Store

M.states = { queued = true, running = true, completed = true, failed = true, cancelled = true }

local function fail(detail)
	errors.raise(errors.new("run.invalid", "Run or event record is invalid", {
		detail = detail,
		remedy = "Persist only validated process, provider, workspace, timing, usage, and credential-free event data.",
	}))
end

local function require_string(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty string")
	end
	return value
end

local function require_identifier(value, name)
	value = require_string(value, name)
	if not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function require_time(value, name)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail(name .. " must be a non-negative integer")
	end
	return value
end

local function validate_fields(value, allowed, name)
	for key in pairs(value) do
		if not allowed[key] then
			fail(name .. " contains unsupported field: " .. tostring(key))
		end
	end
end

local function safe_value(value, path)
	local kind = type(value)
	if kind == "string" or kind == "number" or kind == "boolean" then
		return kind == "string" and redact.text(value) or value
	end
	if kind ~= "table" then
		fail(path .. " must be JSON-compatible")
	end
	local result = {}
	if vim.islist(value) then
		for index, child in ipairs(value) do
			result[index] = safe_value(child, path .. "[" .. index .. "]")
		end
		return result
	end
	for key, child in pairs(value) do
		if type(key) ~= "string" then
			fail(path .. " keys must be strings")
		end
		local normalized = key:lower()
		if
			normalized:match("token")
			or normalized:match("secret")
			or normalized:match("credential")
			or normalized:match("password")
		then
			fail(path .. " must not store credentials")
		end
		result[key] = safe_value(child, path .. "." .. key)
	end
	return result
end

local function provider(value)
	if type(value) ~= "table" then
		fail("provider must be a table")
	end
	validate_fields(value, { name = true, session_id = true }, "provider")
	local result = { name = require_string(value.name, "provider.name") }
	if value.session_id ~= nil then
		result.session_id = require_string(value.session_id, "provider.session_id")
	end
	return result
end

local function process(value)
	if type(value) ~= "table" then
		fail("process must be a table")
	end
	validate_fields(value, { pid = true, executable = true }, "process")
	if type(value.pid) ~= "number" or value.pid < 1 or value.pid % 1 ~= 0 then
		fail("process.pid must be a positive integer")
	end
	return { pid = value.pid, executable = require_string(value.executable, "process.executable") }
end

local function workspace(value)
	if type(value) ~= "table" then
		fail("workspace must be a table")
	end
	validate_fields(value, { kind = true, root = true }, "workspace")
	if value.kind ~= "project" and value.kind ~= "worktree" then
		fail("workspace.kind must be project or worktree")
	end
	return { kind = value.kind, root = require_string(value.root, "workspace.root") }
end

local function timing(value)
	if type(value) ~= "table" then
		fail("timing must be a table")
	end
	validate_fields(value, { started_at = true, ended_at = true, duration_ms = true }, "timing")
	local result = {}
	if value.started_at ~= nil then
		result.started_at = require_time(value.started_at, "timing.started_at")
	end
	if value.ended_at ~= nil then
		result.ended_at = require_time(value.ended_at, "timing.ended_at")
	end
	if result.started_at and result.ended_at and result.ended_at < result.started_at then
		fail("timing.ended_at cannot precede timing.started_at")
	end
	if value.duration_ms ~= nil then
		result.duration_ms = require_time(value.duration_ms, "timing.duration_ms")
	end
	return result
end

local function usage(value)
	if type(value) ~= "table" then
		fail("usage must be a table")
	end
	validate_fields(value, { input_tokens = true, output_tokens = true, total_tokens = true }, "usage")
	local result = {}
	for _, key in ipairs({ "input_tokens", "output_tokens", "total_tokens" }) do
		if value[key] ~= nil then
			result[key] = require_time(value[key], "usage." .. key)
		end
	end
	return result
end

function M.event(attrs)
	if type(attrs) ~= "table" then
		fail("event attributes must be a table")
	end
	validate_fields(attrs, { id = true, run_id = true, type = true, at = true, payload = true }, "event")
	return {
		id = require_identifier(attrs.id, "event.id"),
		run_id = require_identifier(attrs.run_id, "event.run_id"),
		type = require_string(attrs.type, "event.type"),
		at = require_time(attrs.at, "event.at"),
		payload = safe_value(attrs.payload or {}, "event.payload"),
	}
end

local function events(value, run_id)
	if type(value) ~= "table" or not vim.islist(value) then
		fail("events must be an array")
	end
	local result = {}
	for index, event in ipairs(value) do
		result[index] = M.event(event)
		if result[index].run_id ~= run_id then
			fail("event " .. index .. " must belong to its run")
		end
	end
	return result
end

function M.new(attrs)
	if type(attrs) ~= "table" then
		fail("attributes must be a table")
	end
	validate_fields(attrs, {
		id = true,
		task_id = true,
		provider = true,
		process = true,
		workspace = true,
		state = true,
		timing = true,
		usage = true,
		events = true,
	}, "run")
	local id = require_identifier(attrs.id, "id")
	local state = require_string(attrs.state, "state")
	if not M.states[state] then
		fail("state is unknown: " .. state)
	end
	return setmetatable({
		id = id,
		task_id = require_identifier(attrs.task_id, "task_id"),
		provider = provider(attrs.provider),
		process = process(attrs.process),
		workspace = workspace(attrs.workspace),
		state = state,
		timing = timing(attrs.timing),
		usage = usage(attrs.usage),
		events = events(attrs.events or {}, id),
	}, Run)
end

function M.is(value)
	return getmetatable(value) == Run
end

function M.to_record(value)
	if not M.is(value) then
		fail("value must be created by gator.core.run.new")
	end
	local run = M.new(value)
	return {
		id = run.id,
		task_id = run.task_id,
		provider = provider(run.provider),
		process = process(run.process),
		workspace = workspace(run.workspace),
		state = run.state,
		timing = timing(run.timing),
		usage = usage(run.usage),
		events = events(run.events, run.id),
	}
end

function M.from_record(record)
	return M.new(record)
end

function M.append_event(value, event)
	if not M.is(value) then
		fail("run must be created by gator.core.run.new")
	end
	local record = M.to_record(value)
	table.insert(record.events, M.event(event))
	return M.from_record(record)
end

local function records(fs, path)
	if not fs:readable(path) then
		return {}
	end
	local ok, document = pcall(vim.json.decode, fs:read(path))
	if not ok or type(document) ~= "table" then
		fail("run file is not valid JSON: " .. path)
	end
	if document.schema_version ~= 1 or type(document.runs) ~= "table" or not vim.islist(document.runs) then
		fail("run file has an unsupported schema: " .. path)
	end
	return document.runs
end

local function write(fs, path, values)
	local parent = vim.fn.fnamemodify(path, ":h")
	if not fs:mkdir(parent) then
		fail("cannot create run directory: " .. parent)
	end
	local temporary = path .. ".tmp-" .. vim.uv.hrtime()
	local written, write_err = fs:write(temporary, vim.json.encode({ schema_version = 1, runs = values }))
	if not written then
		fail("cannot write run file: " .. tostring(write_err))
	end
	local renamed, rename_err = fs:rename(temporary, path)
	if not renamed then
		fs:remove(temporary)
		fail("cannot replace run file: " .. tostring(rename_err))
	end
end

function M.open(path, opts)
	path = path or vim.fn.stdpath("state") .. "/gator/runs.json"
	if type(path) ~= "string" or path == "" then
		fail("path must be a non-empty string")
	end
	if opts == nil then
		opts = {}
	end
	if type(opts) ~= "table" then
		fail("open options must be a table")
	end
	for key in pairs(opts) do
		if key ~= "filesystem" then
			fail("open options contain unsupported field: " .. tostring(key))
		end
	end
	if opts.filesystem ~= nil and not filesystem.is(opts.filesystem) then
		fail("filesystem must be created by gator.core.filesystem.new")
	end
	return setmetatable({ path = path, filesystem = opts.filesystem or filesystem.new() }, Store)
end

function Store:put(value)
	local run = M.to_record(value)
	local values = records(self.filesystem, self.path)
	local replaced = false
	for index, record in ipairs(values) do
		if M.from_record(record).id == run.id then
			values[index] = run
			replaced = true
		end
	end
	if not replaced then
		table.insert(values, run)
	end
	table.sort(values, function(left, right)
		return M.from_record(left).id < M.from_record(right).id
	end)
	write(self.filesystem, self.path, values)
	return M.from_record(run)
end

function Store:get(id)
	id = require_identifier(id, "id")
	for _, record in ipairs(records(self.filesystem, self.path)) do
		local run = M.from_record(record)
		if run.id == id then
			return run
		end
	end
	return nil
end

function Store:list(task_id)
	if task_id ~= nil then
		require_identifier(task_id, "task_id")
	end
	local result = {}
	for _, record in ipairs(records(self.filesystem, self.path)) do
		local run = M.from_record(record)
		if task_id == nil or run.task_id == task_id then
			table.insert(result, run)
		end
	end
	return result
end

return M
