local lifecycle = require("gator.core.lifecycle")
local task = require("gator.core.task")
local known = { task_template = true, workflow = true, review_action = true }
local M = { api_version = 1, capabilities = vim.deepcopy(known) }

local function fail(message)
	error("Gator workflow SDK: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function safe(value, path)
	if type(value) == "string" or type(value) == "number" or type(value) == "boolean" then
		return value
	end
	if type(value) ~= "table" then
		fail(path .. " must be JSON-compatible")
	end
	local result = {}
	if vim.islist(value) then
		for index, item in ipairs(value) do
			result[index] = safe(item, path .. "[" .. index .. "]")
		end
		return result
	end
	for key, item in pairs(value) do
		if type(key) ~= "string" then
			fail(path .. " keys must be strings")
		end
		local lower = key:lower()
		if lower:match("token") or lower:match("secret") or lower:match("credential") or lower:match("password") then
			fail(path .. " must not include credentials")
		end
		result[key] = safe(item, path .. "." .. key)
	end
	return result
end

local function input(value, label)
	if value == nil then
		return {}
	end
	return safe(value, label .. " input")
end

local function fields(value, allowed, label)
	if type(value) ~= "table" then
		fail(label .. " must be a table")
	end
	for key in pairs(value) do
		if not allowed[key] then
			fail(label .. " contains unsupported field: " .. tostring(key))
		end
	end
end

local function invoke(callback, request, label)
	local ok, result = pcall(callback, request)
	if not ok or not task.is(result) then
		fail(label .. " must return a Gator task")
	end
	return result
end

local function changed(before, result, label)
	local previous = task.to_record(before)
	local next = task.to_record(result)
	if next.id ~= previous.id or next.created_at ~= previous.created_at then
		fail(label .. " must preserve the task identity")
	end
	if not vim.deep_equal(next.sessions, previous.sessions) then
		fail(label .. " must preserve provider-owned sessions")
	end
	if next.updated_at < previous.updated_at then
		fail(label .. " must not move the task timestamp backwards")
	end
	if not lifecycle.can_transition(previous.lifecycle, next.lifecycle) then
		fail(label .. " must use a valid lifecycle transition")
	end
	return result
end

local function capabilities(value)
	if type(value) ~= "table" or not vim.islist(value) or #value == 0 then
		fail("capabilities must be a non-empty list")
	end
	local declared, result = {}, {}
	for index, capability in ipairs(value) do
		if type(capability) ~= "string" or not known[capability] then
			fail("capabilities[" .. index .. "] is unsupported")
		end
		if declared[capability] then
			fail("capabilities must not contain duplicates")
		end
		declared[capability] = true
		table.insert(result, capability)
	end
	return declared, result
end

function M.define(attrs)
	fields(attrs, {
		name = true,
		capabilities = true,
		task_template = true,
		workflow = true,
		review_action = true,
	}, "extension")
	local name = identifier(attrs.name, "extension name")
	local declared, list = capabilities(attrs.capabilities)
	for capability in pairs(known) do
		if declared[capability] and type(attrs[capability]) ~= "function" then
			fail(capability .. " capability requires a callback")
		end
		if not declared[capability] and attrs[capability] ~= nil then
			fail(capability .. " callback requires its capability")
		end
	end
	local extension = { name = name, capabilities = list }

	function extension.task_template(opts)
		if not declared.task_template then
			fail("task_template capability is not declared")
		end
		fields(opts, { input = true }, "task_template request")
		local result = invoke(attrs.task_template, { input = input(opts.input, "task_template") }, "task_template")
		if #task.to_record(result).sessions ~= 0 then
			fail("task_template must not create provider-owned sessions")
		end
		return result
	end

	function extension.workflow(opts)
		if not declared.workflow then
			fail("workflow capability is not declared")
		end
		fields(opts, { task = true, input = true }, "workflow request")
		if not task.is(opts.task) then
			fail("workflow request requires a Gator task")
		end
		local result = invoke(attrs.workflow, {
			task = task.to_record(opts.task),
			input = input(opts.input, "workflow"),
		}, "workflow")
		return changed(opts.task, result, "workflow")
	end

	function extension.review_action(opts)
		if not declared.review_action then
			fail("review_action capability is not declared")
		end
		fields(opts, { task = true, action = true, input = true }, "review_action request")
		if not task.is(opts.task) or opts.task.lifecycle ~= "awaiting_review" then
			fail("review_action requires a task awaiting review")
		end
		local action = identifier(opts.action, "review action")
		local result = invoke(attrs.review_action, {
			task = task.to_record(opts.task),
			action = action,
			input = input(opts.input, "review_action"),
		}, "review_action")
		return changed(opts.task, result, "review_action")
	end

	return extension
end

return M
