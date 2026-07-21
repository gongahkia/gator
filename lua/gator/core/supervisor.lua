local capabilities = require("gator.adapters.capabilities")
local context = require("gator.context.pack")
local errors = require("gator.error")
local event = require("gator.core.run").event
local overlay = require("gator.policy.overlay")
local runtime = require("gator.core.runtime")
local task = require("gator.core.task")
local M = {}
local Supervisor = {}

Supervisor.__index = Supervisor

local function fail(message)
	error("Gator native run contract: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function timestamp(value)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail("now must return a non-negative integer timestamp")
	end
	return value
end

local function validate(opts)
	if type(opts) ~= "table" then
		fail("start requires options")
	end
	for key in pairs(opts) do
		if
			key ~= "id"
			and key ~= "task"
			and key ~= "context"
			and key ~= "policy"
			and key ~= "capabilities"
			and key ~= "capability"
			and key ~= "mode"
			and key ~= "adapter"
			and key ~= "args"
			and key ~= "executable"
			and key ~= "timeout_ms"
			and key ~= "on_event"
		then
			fail("start contains unsupported field: " .. tostring(key))
		end
	end
	if
		not task.is(opts.task)
		or not context.is(opts.context)
		or not overlay.is(opts.policy)
		or not capabilities.is(opts.capabilities)
		or type(opts.adapter) ~= "table"
		or type(opts.adapter.launch) ~= "function"
	then
		fail("start requires task, context, narrowed policy, capabilities, and adapter launch")
	end
	if opts.context.task_id ~= opts.task.id then
		fail("context pack must belong to the task")
	end
	if not opts.task.workspace then
		fail("task must have a workspace")
	end
	local cwd = vim.uv.fs_realpath(opts.task.workspace.root)
	if not cwd or vim.fn.isdirectory(cwd) ~= 1 then
		fail("task workspace must resolve to a directory")
	end
	local id = identifier(opts.id, "id")
	if opts.policy.scope ~= "run" or opts.policy.target ~= id or opts.policy.provenance.source ~= "run-override" then
		fail("policy must be the narrowed run overlay for this run")
	end
	local capability = opts.capability or "transport"
	if not capabilities.domains[capability] then
		fail("capability domain is unknown: " .. tostring(capability))
	end
	capabilities.require(opts.capabilities, capability, opts.mode)
	if opts.args ~= nil and (type(opts.args) ~= "table" or not vim.islist(opts.args)) then
		fail("args must be an array")
	end
	if opts.executable ~= nil and (type(opts.executable) ~= "string" or opts.executable == "") then
		fail("executable must be a non-empty string")
	end
	if
		opts.timeout_ms ~= nil
		and (type(opts.timeout_ms) ~= "number" or opts.timeout_ms < 1 or opts.timeout_ms % 1 ~= 0)
	then
		fail("timeout_ms must be a positive integer")
	end
	if opts.on_event ~= nil and type(opts.on_event) ~= "function" then
		fail("on_event must be a function")
	end
	return { id = id, cwd = cwd, capability = capability }
end

local function normalized(status)
	local output = status.output or { stdout = "", stderr = "", truncated = {} }
	return {
		state = status.state,
		process = { pid = status.pid, executable = status.executable },
		timeout_ms = status.timeout_ms,
		result = status.result and vim.deepcopy(status.result) or nil,
		failure = status.failure and vim.deepcopy(status.failure) or nil,
		output = {
			stdout_bytes = #(output.stdout or ""),
			stderr_bytes = #(output.stderr or ""),
			truncated = vim.deepcopy(output.truncated or {}),
		},
	}
end

function M.new(opts)
	if type(opts) ~= "table" or type(opts.manager) ~= "table" or type(opts.manager.launch) ~= "function" then
		fail("new requires an asynchronous process supervisor")
	end
	for key in pairs(opts) do
		if key ~= "manager" and key ~= "now" and key ~= "runtime" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	if opts.now ~= nil and type(opts.now) ~= "function" then
		fail("now must be a function")
	end
	if opts.runtime ~= nil and not runtime.is(opts.runtime) then
		fail("runtime must be created by gator.core.runtime.new")
	end
	if opts.runtime and opts.now then
		fail("new accepts either runtime or now")
	end
	return setmetatable(
		{ manager = opts.manager, runtime = opts.runtime or runtime.new({ clock = opts.now }) },
		Supervisor
	)
end

function Supervisor:start(opts)
	local value = validate(opts)
	local events, sequence, exited = {}, 0, false
	local function emit(status)
		sequence = sequence + 1
		local record = event({
			id = self.runtime:next_id(value.id .. "-lifecycle"),
			run_id = value.id,
			type = "run." .. status.state,
			at = timestamp(self.runtime:now()),
			payload = normalized(status),
		})
		events[#events + 1] = record
		if opts.on_event then
			opts.on_event(vim.deepcopy(record))
		end
	end
	local managed = {
		launch = function(_, request)
			request = vim.deepcopy(request)
			request.timeout_ms = opts.timeout_ms or request.timeout_ms
			request.on_exit = function(status)
				exited = true
				emit(status)
			end
			return self.manager:launch(request)
		end,
	}
	local request = { manager = managed, id = value.id, cwd = value.cwd }
	if opts.args ~= nil then
		request.args = vim.deepcopy(opts.args)
	end
	if opts.executable ~= nil then
		request.executable = opts.executable
	end
	local launched, status = pcall(opts.adapter.launch, request)
	if not launched then
		errors.raise(errors.runtime("launch_failed", { detail = tostring(status) }))
	end
	if type(status) ~= "table" or status.id ~= value.id or type(status.state) ~= "string" then
		errors.raise(
			errors.runtime("protocol_failed", { detail = "adapter did not return a managed provider-native run" })
		)
	end
	if not exited then
		emit(status)
	end
	return {
		id = value.id,
		task_id = opts.task.id,
		context_id = opts.context.id,
		provider = opts.capabilities.provider,
		capability = value.capability,
		status = vim.deepcopy(status),
		events = events,
	}
end

return M
