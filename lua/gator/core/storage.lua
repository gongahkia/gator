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

return M
