local core = require("gator").module("core")
local thread = core.thread
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local value = thread.new({
	id = "thread-one",
	task_id = "task-thread",
	created_at = 1,
	updated_at = 1,
})
local entry = thread.entry({
	id = "entry-one",
	role = "assistant",
	content = "Normalized provider response",
	created_at = 2,
	provenance = { kind = "provider", provider = "codex", session_id = "native-session-1" },
})
local appended = thread.append(value, entry, 2)

assert(#value.entries == 0, "thread append must not mutate existing threads")
assert(#appended.entries == 1, "thread append must retain normalized entries")
assert(
	appended.entries[1].provenance.session_id == "native-session-1",
	"thread entries must preserve native provenance"
)

local store = thread.open(helpers.tempdir("threads") .. "/threads.json")
store:put(appended)
assert(store:get("thread-one").entries[1].content == "Normalized provider response", "threads must persist locally")
assert(#store:list("task-thread") == 1, "threads must list by task")

local ok = pcall(thread.entry, {
	id = "entry-two",
	role = "assistant",
	content = "invalid",
	created_at = 3,
	provenance = { kind = "provider", provider = "codex", session_id = "native-session-1", history = "rewritten" },
})
assert(not ok, "thread entries must reject provider-native history fields")
ok = pcall(thread.entry, {
	id = "entry-three",
	role = "assistant",
	content = "invalid",
	created_at = 3,
	provenance = { kind = "provider", provider = "codex", session_id = "native-session-1", token = "credential" },
})
assert(not ok, "thread entries must reject provider credential fields")
