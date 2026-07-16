local run = require("gator").module("core").run
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local value = run.new({
	id = "run-one",
	task_id = "task-run",
	provider = { name = "codex", session_id = "native-session-1" },
	process = { pid = 42, executable = "codex" },
	workspace = { kind = "worktree", root = "/tmp/gator-run" },
	state = "running",
	timing = { started_at = 1 },
	usage = { input_tokens = 10 },
})
local event = run.event({
	id = "event-one",
	run_id = "run-one",
	type = "stream.delta",
	at = 2,
	payload = { text = "hello", items = { 1, true } },
})
local streamed = run.append_event(value, event)

assert(run.is(streamed), "runs must have a distinct entity type")
assert(#value.events == 0 and #streamed.events == 1, "streamed events must not mutate prior runs")
assert(streamed.provider.session_id == "native-session-1", "runs must preserve native session provenance")
assert(
	streamed.process.pid == 42 and streamed.workspace.kind == "worktree",
	"runs must preserve process and workspace data"
)
assert(streamed.timing.started_at == 1 and streamed.usage.input_tokens == 10, "runs must preserve timing and usage")

local store = run.open(helpers.tempdir("runs") .. "/runs.json")
store:put(streamed)
assert(store:get("run-one").events[1].payload.text == "hello", "run stores must persist streamed events")
assert(store:get("run-one").events[1].payload.items[2], "run stores must preserve structured event payloads")
assert(#store:list("task-run") == 1, "run stores must list by task")

local ok = pcall(run.event, {
	id = "event-secret",
	run_id = "run-one",
	type = "stream.delta",
	at = 3,
	payload = { token = "credential" },
})
assert(not ok, "event payloads must reject credentials")
ok = pcall(run.append_event, value, {
	id = "event-wrong-run",
	run_id = "run-other",
	type = "stream.delta",
	at = 3,
	payload = {},
})
assert(not ok, "events must not attach to a different run")
