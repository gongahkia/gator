local task = require("gator.core.task")
local M = { api_version = 1 }
local names = { "adapter_sdk", "events", "policy_sdk", "retrieval_sdk", "ui_sdk", "workflow_sdk" }

local function fail(message)
	error("Gator extension contracts: " .. message, 3)
end

local function fields(value, allowed, name)
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

local function expect_failure(callback, name)
	if pcall(callback) then
		fail(name .. " accepted an invalid value")
	end
end

local function selected(value)
	if value == nil then
		return vim.deepcopy(names)
	end
	if type(value) ~= "table" or not vim.islist(value) or #value == 0 then
		fail("only must be a non-empty list")
	end
	local known, result = {}, {}
	for index, name in ipairs(value) do
		if type(name) ~= "string" or not vim.tbl_contains(names, name) then
			fail("only[" .. index .. "] is unsupported")
		end
		if known[name] then
			fail("only must not contain duplicates")
		end
		known[name] = true
		table.insert(result, name)
	end
	table.sort(result)
	return result
end

local function surface(api, name, functions)
	local value = api[name]
	if type(value) ~= "table" or value.api_version ~= 1 then
		fail(name .. " must expose API version 1")
	end
	for _, function_name in ipairs(functions) do
		if type(value[function_name]) ~= "function" then
			fail(name .. " must expose " .. function_name)
		end
	end
	return value
end

local function check_adapter(api)
	local sdk = surface(api, "adapter_sdk", { "event", "define" })
	expect_failure(function()
		sdk.event({ provider = "fixture", type = "message", payload = { token = "secret" } })
	end, "adapter SDK")
end

local function check_events(api)
	local events = surface(api, "events", { "event", "subscribe", "unsubscribe", "emit" })
	expect_failure(function()
		events.event({
			schema_version = 1,
			type = "session.linked",
			at = 1,
			payload = { session = { provider = "fixture", id = "native", owner = "gator" } },
		})
	end, "event API")
end

local function check_policy(api)
	local sdk = surface(api, "policy_sdk", { "define" })
	expect_failure(function()
		sdk.define({ name = "fixture" })
	end, "policy SDK")
end

local function check_retrieval(api)
	local sdk = surface(api, "retrieval_sdk", { "define" })
	expect_failure(function()
		sdk.define({ name = "fixture", kind = "unsupported", retrieve = function() end })
	end, "retrieval SDK")
end

local function check_ui(api)
	local sdk = surface(api, "ui_sdk", { "register", "unregister", "open", "action" })
	expect_failure(function()
		sdk.register({ name = "fixture" })
	end, "UI SDK")
end

local function check_workflow(api)
	local sdk = surface(api, "workflow_sdk", { "define" })
	local template = sdk.define({
		name = "contract-template",
		capabilities = { "task_template" },
		task_template = function()
			return task.new({ id = "contract-task", objective = "Contract", created_at = 1 })
		end,
	})
	expect_failure(function()
		template.task_template({ input = { token = "secret" } })
	end, "workflow SDK")
end

local checks = {
	adapter_sdk = check_adapter,
	events = check_events,
	policy_sdk = check_policy,
	retrieval_sdk = check_retrieval,
	ui_sdk = check_ui,
	workflow_sdk = check_workflow,
}

function M.verify(opts)
	opts = fields(opts, { only = true }, "verify")
	local api = require("gator.extensions")
	local report = { api_version = M.api_version, modules = selected(opts.only) }
	for _, name in ipairs(report.modules) do
		checks[name](api)
	end
	return report
end

return M
