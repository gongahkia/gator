local overlay = require("gator.policy.overlay")
local M = {}

local function fail(message)
	error("Gator review validation: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function workspace(value)
	if type(value) ~= "table" then
		fail("workspace must be a table")
	end
	for key in pairs(value) do
		if key ~= "kind" and key ~= "root" then
			fail("workspace contains unsupported field: " .. tostring(key))
		end
	end
	if value.kind ~= "project" and value.kind ~= "worktree" then
		fail("workspace.kind must be project or worktree")
	end
	if type(value.root) ~= "string" or value.root == "" then
		fail("workspace.root must be an existing directory")
	end
	local root = vim.uv.fs_realpath(value.root)
	if not root or vim.fn.isdirectory(root) ~= 1 then
		fail("workspace.root must be an existing directory")
	end
	return { kind = value.kind, root = root }
end

local function command(policy, command_id)
	if not overlay.is(policy) or type(policy.rules.test_commands) ~= "table" then
		fail("policy must explicitly define test_commands")
	end
	local value = policy.rules.test_commands[identifier(command_id, "command_id")]
	if type(value) ~= "table" then
		fail("command is not policy-approved: " .. command_id)
	end
	for key in pairs(value) do
		if key ~= "argv" then
			fail("approved command contains unsupported field: " .. tostring(key))
		end
	end
	if type(value.argv) ~= "table" or not vim.islist(value.argv) or #value.argv == 0 then
		fail("approved command argv must be a non-empty array")
	end
	local argv = {}
	for index, argument in ipairs(value.argv) do
		if type(argument) ~= "string" or argument == "" then
			fail("approved command argument " .. index .. " must be a non-empty string")
		end
		argv[index] = argument
	end
	return argv
end

local function default_run(argv, cwd, emit)
	local result = vim.system(argv, {
		cwd = cwd,
		text = true,
		stdout = function(_, chunk)
			if chunk and chunk ~= "" then
				emit("stdout", chunk)
			end
		end,
		stderr = function(_, chunk)
			if chunk and chunk ~= "" then
				emit("stderr", chunk)
			end
		end,
	}):wait()
	return { code = result.code }
end

function M.execute(opts)
	if type(opts) ~= "table" then
		fail("execute requires options")
	end
	for key in pairs(opts) do
		if key ~= "policy" and key ~= "command_id" and key ~= "workspace" and key ~= "run" and key ~= "on_evidence" then
			fail("execute contains unsupported field: " .. tostring(key))
		end
	end
	if opts.run ~= nil and type(opts.run) ~= "function" then
		fail("run must be a function")
	end
	if opts.on_evidence ~= nil and type(opts.on_evidence) ~= "function" then
		fail("on_evidence must be a function")
	end
	local command_id = identifier(opts.command_id, "command_id")
	local argv = command(opts.policy, command_id)
	local target = workspace(opts.workspace)
	local evidence = {}
	local function emit(stream, text)
		if (stream ~= "stdout" and stream ~= "stderr") or type(text) ~= "string" or text == "" then
			fail("command evidence must have a stream and non-empty text")
		end
		local value = { stream = stream, text = text }
		table.insert(evidence, value)
		if opts.on_evidence then
			opts.on_evidence(vim.deepcopy(value))
		end
	end
	local ok, result = pcall(opts.run or default_run, argv, target.root, emit)
	if not ok or type(result) ~= "table" or type(result.code) ~= "number" or result.code % 1 ~= 0 then
		fail("policy-approved command failed to launch")
	end
	return {
		command_id = command_id,
		argv = argv,
		workspace = target,
		policy = { scope = opts.policy.scope, provenance = vim.deepcopy(opts.policy.provenance) },
		code = result.code,
		passed = result.code == 0,
		evidence = evidence,
	}
end

return M
