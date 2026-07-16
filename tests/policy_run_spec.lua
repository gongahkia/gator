local core_run = require("gator").module("core").run
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local overlay = require("gator").module("policy").overlay
local policy_run = require("gator").module("policy").run

local run = core_run.new({
	id = "run-policy",
	task_id = "task-policy",
	provider = { name = "codex", session_id = "native-session" },
	process = { pid = 1, executable = "codex" },
	workspace = { kind = "worktree", root = "/tmp/gator-policy" },
	state = "running",
	timing = {},
	usage = {},
})
local baseline = overlay.new({
	scope = "project",
	target = "/tmp/gator-policy",
	rules = { write_allowed = true, network_allowed = true, mode = "default" },
	provenance = { source = "project-policy", ref = ".gator/policy.json" },
})
local override = overlay.new({
	scope = "run",
	target = "run-policy",
	rules = { write_allowed = false, network_allowed = false, mode = "plan" },
	provenance = { source = "run-override", ref = "run-policy" },
})
local store = core_run.open(helpers.tempdir("policy-run") .. "/runs.json")
store:put(run)
local value = policy_run.apply({ run = run, store = store, baseline = baseline, override = override, at = 1 })
assert(
	value.policy.scope == "run"
		and not value.policy.rules.write_allowed
		and not value.policy.rules.network_allowed
		and value.policy.rules.mode == "plan",
	"run policies must apply only explicit narrowing and mode selection"
)
assert(
	#run.events == 0
		and value.run.provider.session_id == "native-session"
		and value.run.events[1].type == "policy.run_override"
		and value.run.events[1].payload.baseline.provenance.source == "project-policy"
		and value.run.events[1].payload.override.provenance.source == "run-override",
	"run overrides must persist provenance without changing provider session ownership"
)
assert(
	store:get("run-policy").events[1].id == value.event.id,
	"run override audit events must persist in the run store"
)
local broader = overlay.new({
	scope = "run",
	target = "run-policy",
	rules = { write_allowed = true },
	provenance = { source = "run-override", ref = "run-policy" },
})
local read_only = overlay.new({
	scope = "project",
	target = "/tmp/gator-policy",
	rules = { write_allowed = false, mode = "read_only" },
	provenance = { source = "project-policy", ref = ".gator/policy.json" },
})
assert(
	not pcall(policy_run.apply, { run = run, store = store, baseline = read_only, override = broader, at = 2 }),
	"run overrides must reject broader permissions"
)
assert(
	not pcall(policy_run.apply, { run = value.run, store = store, baseline = baseline, override = override, at = 1 }),
	"run overrides must reject duplicate audit events"
)
