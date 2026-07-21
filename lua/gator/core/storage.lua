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

return M
