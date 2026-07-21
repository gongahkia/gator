local M = {
	api_version = 1,
	json_backend = {
		api_version = 1,
		methods = {
			"put_task",
			"get_task",
			"list_tasks",
			"query_tasks",
			"append_run",
			"append_run_event",
			"get_run",
			"list_runs",
			"query_runs",
			"append_evidence_excerpt",
			"list_evidence_excerpts",
			"delete_evidence_excerpt",
			"commit_task_operation",
			"list_task_operations",
			"preview_export",
			"export_bundle",
		},
	},
}

local database = require("gator.core.database")
local filesystem = require("gator.core.filesystem")
local json_backend = require("gator.core.json_backend")

local function fail(message)
	error("Gator storage contract: " .. message, 3)
end

function M.validate_json_backend(value)
	if type(value) ~= "table" or value.api_version ~= M.json_backend.api_version then
		fail("JSON backend must expose the current API version")
	end
	for _, name in ipairs(M.json_backend.methods) do
		if type(value[name]) ~= "function" then
			fail("JSON backend must expose " .. name)
		end
	end
	return value
end

function M.json_contract()
	return vim.deepcopy(M.json_backend)
end

function M.resolve(opts)
	if opts == nil then
		opts = {}
	end
	if type(opts) ~= "table" then
		fail("resolve requires options")
	end
	for key in pairs(opts) do
		if key ~= "kind" and key ~= "path" and key ~= "filesystem" then
			fail("resolve contains unsupported field: " .. tostring(key))
		end
	end
	local kind = opts.kind or "sqlite"
	if kind ~= "sqlite" and kind ~= "json" then
		fail("storage backend is unavailable: " .. tostring(kind))
	end
	if opts.path ~= nil and (type(opts.path) ~= "string" or opts.path == "") then
		fail("storage path must be non-empty")
	end
	if opts.filesystem ~= nil and not filesystem.is(opts.filesystem) then
		fail("filesystem must be a Gator filesystem")
	end
	if kind == "sqlite" then
		if opts.filesystem then
			fail("SQLite storage cannot use a JSON filesystem boundary")
		end
		return { kind = kind, backend = database.open(opts.path) }
	end
	local path = opts.path or vim.fn.stdpath("state") .. "/gator/state.json"
	return { kind = kind, backend = json_backend.open(path, { filesystem = opts.filesystem }) }
end

function M.migrate_sqlite_to_json(opts)
	if type(opts) ~= "table" then
		fail("migration requires options")
	end
	for key in pairs(opts) do
		if key ~= "source" and key ~= "target" and key ~= "confirm" then
			fail("migration contains unsupported field: " .. tostring(key))
		end
	end
	if opts.confirm ~= true then
		fail("migration requires explicit confirmation")
	end
	if type(opts.source) ~= "table" or type(opts.source.preview_export) ~= "function" then
		fail("migration source must expose an export preview")
	end
	M.validate_json_backend(opts.target)
	if type(opts.target.read) ~= "function" or type(opts.target.write) ~= "function" then
		fail("migration target must expose atomic document access")
	end
	local target = opts.target:read()
	if #target.tasks > 0 or #target.runs > 0 or #target.evidence_excerpts > 0 or #target.operations > 0 then
		fail("migration target must be empty")
	end
	local preview = opts.source:preview_export()
	if type(preview) ~= "table" or preview.schema_version ~= M.json_backend.api_version then
		fail("migration source preview has an unsupported schema")
	end
	for _, key in ipairs({ "tasks", "runs", "evidence_excerpts", "operations" }) do
		if type(preview[key]) ~= "table" or not vim.islist(preview[key]) then
			fail("migration source preview contains invalid " .. key)
		end
	end
	opts.target:write(vim.deepcopy(preview))
	return opts.target:preview_export()
end

function M.migrate_json_to_sqlite(opts)
	if type(opts) ~= "table" then
		fail("migration requires options")
	end
	for key in pairs(opts) do
		if key ~= "source" and key ~= "target" and key ~= "confirm" then
			fail("migration contains unsupported field: " .. tostring(key))
		end
	end
	if opts.confirm ~= true then
		fail("migration requires explicit confirmation")
	end
	M.validate_json_backend(opts.source)
	if type(opts.target) ~= "table" then
		fail("migration target must be a SQLite database")
	end
	for _, name in ipairs({ "migrate", "version", "import_bundle" }) do
		if type(opts.target[name]) ~= "function" then
			fail("migration target must expose " .. name)
		end
	end
	local preview = opts.source:preview_export()
	if type(preview) ~= "table" or preview.schema_version ~= M.json_backend.api_version then
		fail("migration source preview has an unsupported schema")
	end
	for _, key in ipairs({ "tasks", "runs", "evidence_excerpts", "operations" }) do
		if type(preview[key]) ~= "table" or not vim.islist(preview[key]) then
			fail("migration source preview contains invalid " .. key)
		end
	end
	opts.target:migrate()
	if opts.target:version() ~= database.schema_version then
		fail("migration target has an unsupported SQLite schema")
	end
	return opts.target:import_bundle(vim.deepcopy(preview))
end

return M
