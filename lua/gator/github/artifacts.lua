local redact = require("gator.policy.redact")
local task = require("gator.core.task")
local M = {}
local directory = ".gator/tasks"

local function fail(message)
	error("Gator shared artifacts: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function invoke(run, argv, cwd, name)
	local ok, result = pcall(run, argv, cwd)
	if not ok or type(result) ~= "table" or type(result.code) ~= "number" or result.code % 1 ~= 0 then
		fail(name .. " failed")
	end
	if result.stdout ~= nil and type(result.stdout) ~= "string" then
		fail(name .. " returned invalid stdout")
	end
	return { code = result.code, stdout = result.stdout or "" }
end

local function plan(value)
	if value == nil then
		return {}
	end
	if type(value) ~= "table" or not vim.islist(value) then
		fail("plan must be a list")
	end
	local result = {}
	for index, step in ipairs(value) do
		if type(step) ~= "string" or step == "" then
			fail("plan[" .. index .. "] must be a non-empty string")
		end
		result[index] = redact.text(step)
	end
	return result
end

local function handoff(value)
	if value == nil then
		return nil
	end
	if type(value) ~= "string" or value == "" then
		fail("handoff must be a non-empty string")
	end
	return redact.text(value)
end

local function review(value, task_id)
	if value == nil then
		return nil
	end
	if type(value) ~= "table" or value.task_id ~= task_id then
		fail("review_evidence must belong to the task")
	end
	return redact.value(value)
end

local function document(value, opts)
	local record = task.to_record(value)
	local result = {
		schema_version = 1,
		task = {
			id = record.id,
			objective = redact.text(record.objective),
			lifecycle = record.lifecycle,
			evidence = redact.value(record.evidence),
		},
		plan = plan(opts.plan),
	}
	local next_handoff = handoff(opts.handoff)
	if next_handoff then
		result.handoff = next_handoff
	end
	local next_review = review(opts.review_evidence, record.id)
	if next_review then
		result.review_evidence = next_review
	end
	return result
end

local function write(path, value)
	local parent = vim.fn.fnamemodify(path, ":h")
	if vim.fn.mkdir(parent, "p") < 0 then
		fail("cannot create shared artifact directory: " .. parent)
	end
	local temporary = path .. ".tmp-" .. vim.uv.hrtime()
	local ok, err = pcall(vim.fn.writefile, { vim.json.encode(value) }, temporary)
	if not ok then
		fail("cannot write shared artifact: " .. err)
	end
	local renamed, rename_err = vim.uv.fs_rename(temporary, path)
	if not renamed then
		vim.fn.delete(temporary)
		fail("cannot replace shared artifact: " .. rename_err)
	end
end

function M.write(opts)
	if type(opts) ~= "table" then
		fail("write requires options")
	end
	for key in pairs(opts) do
		if
			key ~= "cwd"
			and key ~= "task"
			and key ~= "plan"
			and key ~= "handoff"
			and key ~= "review_evidence"
			and key ~= "opt_in"
			and key ~= "run"
		then
			fail("write contains unsupported field: " .. tostring(key))
		end
	end
	if opts.opt_in ~= true then
		fail("shared artifact writing requires explicit opt_in")
	end
	if type(opts.cwd) ~= "string" or opts.cwd == "" then
		fail("cwd must be an existing directory")
	end
	if not task.is(opts.task) then
		fail("task must be a persistent Gator task")
	end
	if opts.run ~= nil and type(opts.run) ~= "function" then
		fail("run must be a function")
	end
	local cwd = vim.uv.fs_realpath(opts.cwd)
	if not cwd or vim.fn.isdirectory(cwd) ~= 1 then
		fail("cwd must be an existing directory")
	end
	local run = opts.run
		or function(argv, path)
			local result = vim.system(argv, { cwd = path, text = true }):wait()
			return { code = result.code, stdout = result.stdout or "" }
		end
	local root_result = invoke(run, { "git", "rev-parse", "--show-toplevel" }, cwd, "Git root lookup")
	if root_result.code ~= 0 then
		fail("Git root lookup failed")
	end
	local root = vim.uv.fs_realpath(vim.trim(root_result.stdout))
	if not root then
		fail("Git root lookup returned an unavailable path")
	end
	local id = identifier(opts.task.id, "task.id")
	local ref = directory .. "/" .. id .. ".json"
	local ignored = invoke(run, { "git", "check-ignore", "--quiet", "--", ref }, root, "Git artifact ignore check")
	if ignored.code == 0 then
		fail("shared artifact path is ignored by Git")
	end
	if ignored.code ~= 1 then
		fail("Git artifact ignore check failed")
	end
	local value = document(opts.task, opts)
	local path = root .. "/" .. ref
	write(path, value)
	return { path = path, ref = ref, document = vim.deepcopy(value) }
end

return M
