local capabilities = require("gator.adapters.capabilities")
local overlay = require("gator.policy.overlay")
local task = require("gator.core.task")
local M = {}

local function fail(message)
	error("Gator review readonly: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function diff(value)
	if type(value) ~= "table" then
		fail("diff must be a table")
	end
	for key in pairs(value) do
		if key ~= "root" and key ~= "base" and key ~= "patch" then
			fail("diff contains unsupported field: " .. tostring(key))
		end
	end
	if
		type(value.root) ~= "string"
		or value.root == ""
		or type(value.base) ~= "string"
		or value.base == ""
		or type(value.patch) ~= "string"
		or value.patch == ""
	then
		fail("diff requires root, base, and patch")
	end
	local root = vim.uv.fs_realpath(value.root)
	if not root or vim.fn.isdirectory(root) ~= 1 then
		fail("diff.root must be an existing directory")
	end
	return { root = root, base = value.base, patch = value.patch }
end

function M.launch(opts)
	if
		type(opts) ~= "table"
		or not task.is(opts.task)
		or not overlay.is(opts.policy)
		or not capabilities.is(opts.capabilities)
	then
		fail("launch requires a task, policy overlay, and capability contract")
	end
	for key in pairs(opts) do
		if
			key ~= "id"
			and key ~= "task"
			and key ~= "diff"
			and key ~= "policy"
			and key ~= "capabilities"
			and key ~= "map_policy"
			and key ~= "is_read_only"
			and key ~= "launch"
		then
			fail("launch contains unsupported field: " .. tostring(key))
		end
	end
	if
		type(opts.map_policy) ~= "function"
		or type(opts.is_read_only) ~= "function"
		or type(opts.launch) ~= "function"
	then
		fail("launch requires policy mapping, read-only verification, and adapter launch callbacks")
	end
	if opts.policy.rules.write_allowed ~= false then
		fail("review policy must explicitly deny writes")
	end
	for key in pairs(opts.policy.rules) do
		if key ~= "write_allowed" then
			fail("review policy contains unsupported rule: " .. key)
		end
	end
	if not opts.task.workspace then
		fail("review task must have a workspace")
	end
	local target = diff(opts.diff)
	local task_root = vim.uv.fs_realpath(opts.task.workspace.root)
	if task_root ~= target.root then
		fail("diff workspace must match the task workspace")
	end
	local ok, mapped = pcall(opts.map_policy, opts.policy, opts.capabilities)
	if not ok or type(mapped) ~= "table" then
		fail("provider policy mapping failed")
	end
	local verified, read_only = pcall(opts.is_read_only, vim.deepcopy(mapped), opts.capabilities)
	if not verified or read_only ~= true then
		fail("provider policy does not prove read-only access")
	end
	local request = {
		id = identifier(opts.id, "id"),
		provider = opts.capabilities.provider,
		task_id = opts.task.id,
		cwd = target.root,
		policy = vim.deepcopy(mapped),
		review = { base_revision = target.base, patch = target.patch },
	}
	local launched, result = pcall(opts.launch, vim.deepcopy(request))
	if
		not launched
		or type(result) ~= "table"
		or result.id ~= request.id
		or type(result.state) ~= "string"
		or result.state == ""
	then
		fail("read-only reviewer launch failed")
	end
	for key in pairs(result) do
		if key ~= "id" and key ~= "state" then
			fail("reviewer launch must not return session or credential data")
		end
	end
	return {
		id = result.id,
		state = result.state,
		task_id = opts.task.id,
		provider = request.provider,
		policy = request.policy,
	}
end

return M
