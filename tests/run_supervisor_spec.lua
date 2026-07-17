local adapters = require("gator").module("adapters")
local context = require("gator").module("context").pack
local overlay = require("gator").module("policy").overlay
local process = adapters.process
local supervisor = require("gator").module("core").supervisor
local task = require("gator").module("core").task
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local supported = { available = true, modes = { "native" } }
local contract = adapters.capabilities.new({
	provider = "codex",
	transport = { available = true, modes = { "jsonrpc" } },
	auth = supported,
	session = supported,
	permission = supported,
	model = supported,
	command = supported,
	tool = supported,
	context = supported,
	usage = supported,
})
local root = helpers.tempdir("run-supervisor")
local entity = task.new({
	id = "task-supervisor",
	objective = "Run a provider-native agent",
	workspace = { kind = "project", root = root },
	created_at = 1,
	updated_at = 1,
})
local pack = context.new({ id = "pack-supervisor", task_id = entity.id, entries = {} })
local policy = overlay.new({
	scope = "run",
	target = "run-supervisor",
	rules = { write_allowed = false },
	provenance = { source = "run-override", ref = "run-supervisor" },
})
local callback
local manager = process.new({
	shutdown = false,
	spawn = function(_, opts, exit)
		callback = exit
		opts.stdout("provider output")
		return {
			pid = 42,
			kill = function()
				return true
			end,
		}
	end,
})
local events = {}
local native = supervisor.new({
	manager = manager,
	now = function()
		return 7
	end,
})
local value = native:start({
	id = "run-supervisor",
	task = entity,
	context = pack,
	policy = policy,
	capabilities = contract,
	capability = "transport",
	mode = "jsonrpc",
	adapter = {
		launch = function(request)
			assert(
				request.id == "run-supervisor" and request.cwd == root and request.manager,
				"native adapter launch must receive only the validated run identity and workspace"
			)
			return request.manager:launch({ id = request.id, command = { "codex", "app-server" }, cwd = request.cwd })
		end,
	},
	on_event = function(event)
		table.insert(events, event)
	end,
})
assert(
	value.status.state == "running"
		and value.provider == "codex"
		and value.events[1].type == "run.running"
		and value.events[1].payload.output.stdout_bytes == #"provider output",
	"native run contract must normalize validated provider launches without persisting raw output"
)
callback({ code = 1, signal = 0 })
assert(
	#events == 2
		and events[2].type == "run.failed"
		and events[2].payload.failure.kind == "exit"
		and events[2].payload.output.stdout_bytes == #"provider output",
	"native process exits must produce typed lifecycle events"
)
assert(not pcall(native.start, native, {
	id = "run-unsupported",
	task = entity,
	context = pack,
	policy = overlay.new({
		scope = "run",
		target = "run-unsupported",
		rules = { write_allowed = false },
		provenance = { source = "run-override", ref = "run-unsupported" },
	}),
	capabilities = contract,
	capability = "transport",
	mode = "stdio",
	adapter = { launch = function() end },
}), "unsupported capability mappings must fail before a provider launch")
assert(not pcall(native.start, native, {
	id = "run-policy",
	task = entity,
	context = pack,
	policy = overlay.new({
		scope = "run",
		target = "run-policy",
		rules = { write_allowed = true },
		provenance = { source = "project-policy", ref = "policy" },
	}),
	capabilities = contract,
	capability = "transport",
	mode = "jsonrpc",
	adapter = { launch = function() end },
}), "run contract must refuse policies not proven through narrowed run overrides")
