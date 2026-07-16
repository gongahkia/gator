local core = require("gator").module("core")
local metadata = core.session_metadata
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local path = helpers.tempdir("session-metadata") .. "/sessions.json"
local store = metadata.open(path)
local value = metadata.new({
	task_id = "task-metadata",
	provider = "codex",
	id = "native-session-1",
	owner = "provider",
	provider_version = "1.2.3",
	capabilities = { resume = true, structured_output = false },
	probed_at = 1,
})

store:put(value)
assert(vim.fn.filereadable(path) == 1, "session metadata must persist in local state")
local restored = metadata.open(path):get("task-metadata", "codex", "native-session-1")
assert(metadata.is(restored), "stored metadata must restore its entity type")
assert(restored.provider_version == "1.2.3", "stored metadata must preserve provider version provenance")
assert(restored.capabilities.resume, "stored metadata must preserve capability provenance")
assert(#store:list("task-metadata") == 1, "stored metadata must list by task")

local updated = metadata.new({
	task_id = "task-metadata",
	provider = "codex",
	id = "native-session-1",
	owner = "provider",
	provider_version = "1.2.4",
	capabilities = { resume = true, structured_output = true },
	probed_at = 2,
})
store:put(updated)
assert(#store:list() == 1, "metadata updates must replace the same native session")
assert(
	store:get("task-metadata", "codex", "native-session-1").provider_version == "1.2.4",
	"metadata updates must persist"
)

local ok = pcall(metadata.new, {
	task_id = "task-metadata",
	provider = "codex",
	id = "native-session-2",
	owner = "provider",
	provider_version = "1.2.4",
	capabilities = {},
	probed_at = 2,
	token = "credential",
})
assert(not ok, "session metadata must reject credential fields")

local corrupt = helpers.tempdir("corrupt-session-metadata") .. "/sessions.json"
helpers.write(corrupt, "not json")
ok = pcall(function()
	metadata.open(corrupt):list()
end)
assert(not ok, "corrupt local metadata must fail explicitly")
