local gh = require("gator.github.gh")
local redact = require("gator.policy.redact")
local M = {}

local function fail(message)
	error("Gator GitHub publishing: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function number(value)
	if type(value) ~= "number" or value < 1 or value % 1 ~= 0 then
		fail("number must be a positive integer")
	end
	return value
end

local function text(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty string")
	end
	return value
end

local function timestamp(value, name)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail(name .. " must be a non-negative integer timestamp")
	end
	return value
end

local function record(value, index)
	if type(value) ~= "table" then
		fail("records[" .. index .. "] must be an object")
	end
	local kind = value.kind
	if kind == "reviewer" then
		for key in pairs(value) do
			if key ~= "task_id" and key ~= "kind" and key ~= "reviewer" then
				fail("reviewer evidence contains unsupported field: " .. tostring(key))
			end
		end
		return {
			task_id = identifier(value.task_id, "task_id"),
			kind = kind,
			reviewer = text(value.reviewer, "reviewer"),
		}
	end
	if kind == "test" then
		for key in pairs(value) do
			if
				key ~= "task_id"
				and key ~= "kind"
				and key ~= "command_id"
				and key ~= "output_ref"
				and key ~= "passed"
				and key ~= "at"
			then
				fail("test evidence contains unsupported field: " .. tostring(key))
			end
		end
		if type(value.passed) ~= "boolean" then
			fail("test evidence passed must be boolean")
		end
		return {
			task_id = identifier(value.task_id, "task_id"),
			kind = kind,
			command_id = identifier(value.command_id, "command_id"),
			output_ref = text(value.output_ref, "output_ref"),
			passed = value.passed,
			at = timestamp(value.at, "at"),
		}
	end
	if kind == "approval" then
		for key in pairs(value) do
			if key ~= "task_id" and key ~= "kind" and key ~= "reviewer" and key ~= "approved" and key ~= "at" then
				fail("approval evidence contains unsupported field: " .. tostring(key))
			end
		end
		if type(value.approved) ~= "boolean" then
			fail("approval evidence approved must be boolean")
		end
		return {
			task_id = identifier(value.task_id, "task_id"),
			kind = kind,
			reviewer = text(value.reviewer, "reviewer"),
			approved = value.approved,
			at = timestamp(value.at, "at"),
		}
	end
	if kind == "revision" then
		for key in pairs(value) do
			if
				key ~= "task_id"
				and key ~= "kind"
				and key ~= "base_revision"
				and key ~= "head_revision"
				and key ~= "at"
			then
				fail("revision evidence contains unsupported field: " .. tostring(key))
			end
		end
		return {
			task_id = identifier(value.task_id, "task_id"),
			kind = kind,
			base_revision = text(value.base_revision, "base_revision"),
			head_revision = text(value.head_revision, "head_revision"),
			at = timestamp(value.at, "at"),
		}
	end
	fail("records[" .. index .. "] has an unsupported evidence kind")
end

local function records(value)
	if type(value) ~= "table" or not vim.islist(value) or #value == 0 then
		fail("records must be a non-empty list")
	end
	local result, task_id = {}, nil
	for index, value in ipairs(value) do
		result[index] = record(value, index)
		if task_id and result[index].task_id ~= task_id then
			fail("records must belong to one task")
		end
		task_id = result[index].task_id
	end
	return result, task_id
end

local function body(task_id, records)
	local lines = { "## Gator review evidence", "", "Task: `" .. task_id .. "`", "" }
	for _, value in ipairs(records) do
		if value.kind == "reviewer" then
			table.insert(lines, "- Reviewer: `" .. value.reviewer .. "`")
		elseif value.kind == "test" then
			table.insert(lines, "- Test `" .. value.command_id .. "`: " .. (value.passed and "passed" or "failed"))
			table.insert(lines, "  Output: `" .. value.output_ref .. "`")
		elseif value.kind == "approval" then
			table.insert(
				lines,
				"- Approval by `" .. value.reviewer .. "`: " .. (value.approved and "approved" or "not approved")
			)
		else
			table.insert(lines, "- Revision: `" .. value.base_revision .. "` → `" .. value.head_revision .. "`")
		end
	end
	return redact.text(table.concat(lines, "\n"))
end

local function invoke(run, argv, cwd)
	local ok, result = pcall(run, argv, cwd)
	if not ok or type(result) ~= "table" or type(result.code) ~= "number" or result.code % 1 ~= 0 then
		fail("GitHub pull request comment failed")
	end
	if result.stdout ~= nil and type(result.stdout) ~= "string" then
		fail("GitHub pull request comment returned invalid stdout")
	end
	return { code = result.code, stdout = result.stdout or "" }
end

function M.review_evidence(opts)
	if type(opts) ~= "table" then
		fail("review_evidence requires options")
	end
	for key in pairs(opts) do
		if key ~= "number" and key ~= "records" and key ~= "confirm" and key ~= "cwd" and key ~= "run" then
			fail("review_evidence contains unsupported field: " .. tostring(key))
		end
	end
	if opts.confirm ~= true then
		fail("publishing review evidence requires explicit confirmation")
	end
	local pull_number = number(opts.number)
	local selected, task_id = records(opts.records)
	if opts.cwd ~= nil and (type(opts.cwd) ~= "string" or opts.cwd == "") then
		fail("cwd must be a non-empty string")
	end
	if type(opts.run) ~= "function" then
		fail("run must be a function")
	end
	local capability = gh.detect({ cwd = opts.cwd, run = opts.run })
	if not capability.available then
		fail(capability.reason)
	end
	if not capability.capabilities.pull_request_import.available then
		fail(capability.capabilities.pull_request_import.reason)
	end
	local value = body(task_id, selected)
	local result = invoke(opts.run, { "gh", "pr", "comment", tostring(pull_number), "--body", value }, opts.cwd)
	if result.code ~= 0 then
		fail("GitHub pull request comment failed")
	end
	return { published = true, task_id = task_id, body = value, output = redact.text(result.stdout) }
end

return M
