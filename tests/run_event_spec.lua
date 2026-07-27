local event = require("gator").module("core").run_event
local run_store = require("gator.run_store")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local root = helpers.tempdir("run-event")
assert(vim.system({ "git", "init", "-q" }, { cwd = root, text = true }):wait().code == 0, "fixture must initialize Git")
local store = run_store.new(root)
store:put({
	id = "run-event",
	provider = "pi",
	role = "primary",
	transport = "terminal",
	state = "completed",
	workspace = { kind = "project", root = root },
	bundle_id = "bundle-event",
	objective = "Explain this failure",
	transcript = "unavailable",
	usage = { state = "unknown", context_tokens_estimate = 1 },
	budget = { limit_tokens = 0, action = "warn", state = "unbounded" },
	created_at = 1,
	updated_at = 1,
})
local first = store:append_event("run-event", "run.created", { role = "primary" }, 1)
local second = store:append_event("run-event", "context.prepared", {
	purpose = "launch",
	provider = "pi",
	transport = "terminal",
	bytes = 32,
	tokens = 8,
	redactions = 1,
	artifacts = { { kind = "selection", path = "src/main.lua", first_line = 1, last_line = 2, bytes = 12 } },
}, 2)
assert(
	first.sequence == 0
		and second.sequence == 1
		and #store:events("run-event") == 2
		and store:events("run-event")[2].payload.artifacts[1].path == "src/main.lua",
	"run events must append in durable sequence order with metadata only"
)
assert(
	not pcall(event.new, {
		schema_version = 1,
		id = "run-event-event-2",
		run_id = "run-event",
		sequence = 2,
		type = "context.sent",
		at = 3,
		payload = { prompt = "do not persist raw context" },
	}),
	"run events must reject raw prompts, code, diffs, transcripts, and outputs"
)
assert(
	store:forget_run("run-event")
		and vim.fn.filereadable(root .. "/.gator/events/run-event.jsonl") == 0,
	"forget must remove a run's Gator-owned immutable journal with its other local artifacts"
)
